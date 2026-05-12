package stripe

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/synthify/backend/packages/shared/domain"
)

const defaultAPIBase = "https://api.stripe.com"

type Config struct {
	SecretKey       string
	WebhookSecret   string
	ProPriceID      string
	ProPriceIDJPY   string
	ProPriceIDUSD   string
	DefaultCurrency string
	SuccessURL      string
	CancelURL       string
	PortalReturnURL string
	APIBase         string
}

type Price struct {
	Plan        domain.BillingPlan
	Currency    domain.BillingCurrency
	Interval    domain.BillingInterval
	StripeID    string
	AmountMinor int64
}

type Provider struct {
	cfg             Config
	pricesByKey     map[string]Price
	pricesByID      map[string]Price
	defaultCurrency domain.BillingCurrency
	client          *http.Client
	now             func() time.Time
}

func NewProvider(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, domain.ErrBillingProviderNotConfigured
	}
	pricesByKey, pricesByID, defaultCurrency, err := buildPriceCatalog(cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.SuccessURL) == "" {
		return nil, fmt.Errorf("%w: BILLING_SUCCESS_URL is required", domain.ErrBillingProviderMisconfigured)
	}
	if strings.TrimSpace(cfg.CancelURL) == "" {
		return nil, fmt.Errorf("%w: BILLING_CANCEL_URL is required", domain.ErrBillingProviderMisconfigured)
	}
	if strings.TrimSpace(cfg.PortalReturnURL) == "" {
		return nil, fmt.Errorf("%w: BILLING_PORTAL_RETURN_URL is required", domain.ErrBillingProviderMisconfigured)
	}
	if cfg.APIBase == "" {
		cfg.APIBase = defaultAPIBase
	}
	return &Provider{
		cfg:             cfg,
		pricesByKey:     pricesByKey,
		pricesByID:      pricesByID,
		defaultCurrency: defaultCurrency,
		client:          http.DefaultClient,
		now:             time.Now,
	}, nil
}

func buildPriceCatalog(cfg Config) (map[string]Price, map[string]Price, domain.BillingCurrency, error) {
	defaultCurrency := domain.BillingCurrency(strings.ToLower(strings.TrimSpace(cfg.DefaultCurrency)))
	if defaultCurrency == "" {
		defaultCurrency = domain.BillingCurrencyJPY
	}
	if err := defaultCurrency.Validate(); err != nil {
		return nil, nil, "", err
	}
	prices := []Price{
		{
			Plan:     domain.BillingPlanPro,
			Currency: domain.BillingCurrencyJPY,
			Interval: domain.BillingIntervalMonth,
			StripeID: strings.TrimSpace(cfg.ProPriceIDJPY),
		},
		{
			Plan:     domain.BillingPlanPro,
			Currency: domain.BillingCurrencyUSD,
			Interval: domain.BillingIntervalMonth,
			StripeID: strings.TrimSpace(cfg.ProPriceIDUSD),
		},
	}
	if cfg.ProPriceID != "" {
		for i := range prices {
			if prices[i].Currency == defaultCurrency && prices[i].StripeID == "" {
				prices[i].StripeID = strings.TrimSpace(cfg.ProPriceID)
			}
		}
	}
	byKey := make(map[string]Price)
	byID := make(map[string]Price)
	for _, price := range prices {
		if price.StripeID == "" {
			continue
		}
		byKey[priceKey(price.Plan, price.Currency)] = price
		byID[price.StripeID] = price
	}
	if len(byKey) == 0 {
		return nil, nil, "", fmt.Errorf("%w: STRIPE_PRO_PRICE_ID_JPY or STRIPE_PRO_PRICE_ID_USD is required", domain.ErrBillingProviderMisconfigured)
	}
	return byKey, byID, defaultCurrency, nil
}

func priceKey(plan domain.BillingPlan, currency domain.BillingCurrency) string {
	return string(plan) + ":" + string(currency)
}

func (p *Provider) EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error) {
	if account.StripeCustomerID != "" {
		return &domain.BillingCustomerRef{ExternalCustomerID: account.StripeCustomerID}, nil
	}
	form := url.Values{}
	form.Set("name", account.Name)
	form.Set("metadata[account_id]", account.AccountID)
	var res struct {
		ID string `json:"id"`
	}
	if err := p.postForm(ctx, "/v1/customers", form, "customer:"+account.AccountID, &res); err != nil {
		return nil, err
	}
	if res.ID == "" {
		return nil, fmt.Errorf("%w: customer response missing id", domain.ErrBillingProviderMisconfigured)
	}
	return &domain.BillingCustomerRef{ExternalCustomerID: res.ID}, nil
}

func (p *Provider) CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error) {
	if plan != domain.BillingPlanPro {
		return nil, fmt.Errorf("%w: %s", domain.ErrBillingPlanInvalid, plan)
	}
	if account.StripeCustomerID == "" {
		return nil, fmt.Errorf("%w: account has no Stripe customer", domain.ErrBillingProviderMisconfigured)
	}
	price, err := p.resolvePrice(plan, currency)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("customer", account.StripeCustomerID)
	form.Set("line_items[0][price]", price.StripeID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", p.cfg.SuccessURL)
	form.Set("cancel_url", p.cfg.CancelURL)
	form.Set("metadata[account_id]", account.AccountID)
	form.Set("metadata[currency]", string(price.Currency))
	form.Set("subscription_data[metadata][account_id]", account.AccountID)
	form.Set("subscription_data[metadata][currency]", string(price.Currency))
	form.Set("allow_promotion_codes", "true")
	var res struct {
		URL string `json:"url"`
	}
	if err := p.postForm(ctx, "/v1/checkout/sessions", form, "checkout:"+account.AccountID+":"+string(plan)+":"+string(price.Currency), &res); err != nil {
		return nil, err
	}
	if res.URL == "" {
		return nil, fmt.Errorf("%w: checkout session response missing url", domain.ErrBillingProviderMisconfigured)
	}
	return &domain.BillingCheckoutSession{URL: res.URL}, nil
}

func (p *Provider) resolvePrice(plan domain.BillingPlan, currency domain.BillingCurrency) (Price, error) {
	if currency == "" {
		currency = p.defaultCurrency
	}
	if err := currency.Validate(); err != nil {
		return Price{}, err
	}
	price, ok := p.pricesByKey[priceKey(plan, currency)]
	if !ok {
		return Price{}, fmt.Errorf("%w: %s", domain.ErrBillingCurrencyUnsupported, currency)
	}
	return price, nil
}

func (p *Provider) CreatePortalSession(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error) {
	if account.StripeCustomerID == "" {
		return nil, fmt.Errorf("%w: account has no Stripe customer", domain.ErrBillingProviderMisconfigured)
	}
	form := url.Values{}
	form.Set("customer", account.StripeCustomerID)
	form.Set("return_url", p.cfg.PortalReturnURL)
	var res struct {
		URL string `json:"url"`
	}
	if err := p.postForm(ctx, "/v1/billing_portal/sessions", form, "portal:"+account.AccountID+":"+strconv.FormatInt(p.now().Unix()/60, 10), &res); err != nil {
		return nil, err
	}
	if res.URL == "" {
		return nil, fmt.Errorf("%w: portal session response missing url", domain.ErrBillingProviderMisconfigured)
	}
	return &domain.BillingPortalSession{URL: res.URL}, nil
}

func (p *Provider) ParseWebhook(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error) {
	if strings.TrimSpace(p.cfg.WebhookSecret) == "" {
		return nil, fmt.Errorf("%w: STRIPE_WEBHOOK_SECRET is required", domain.ErrBillingProviderMisconfigured)
	}
	if err := p.verifySignature(payload, signature); err != nil {
		return nil, err
	}
	var event stripeEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	obj := event.Data.Object
	switch event.Type {
	case "checkout.session.completed":
		return &domain.ProviderWebhookEvent{
			Provider:               "stripe",
			EventID:                event.ID,
			EventType:              event.Type,
			AccountID:              metadataValue(obj.Metadata, "account_id"),
			ExternalCustomerID:     obj.Customer,
			ExternalSubscriptionID: obj.Subscription,
			Status:                 domain.BillingStatusCheckoutPending,
		}, nil
	case "invoice.paid", "invoice.payment_succeeded":
		return p.providerEventFromPrice(event, obj, p.priceFromObject(obj), domain.BillingStatusActive), nil
	case "invoice.payment_failed":
		return p.providerEventFromPrice(event, obj, p.priceFromObject(obj), domain.BillingStatusPastDue), nil
	case "customer.subscription.created", "customer.subscription.updated":
		return p.providerEventFromPrice(event, obj, p.priceFromObject(obj), subscriptionStatus(obj.Status)), nil
	case "customer.subscription.deleted":
		return &domain.ProviderWebhookEvent{
			Provider:               "stripe",
			EventID:                event.ID,
			EventType:              event.Type,
			AccountID:              metadataValue(obj.Metadata, "account_id"),
			ExternalCustomerID:     obj.Customer,
			ExternalSubscriptionID: "",
			Plan:                   domain.BillingPlanFree,
			Status:                 domain.BillingStatusFree,
		}, nil
	default:
		return &domain.ProviderWebhookEvent{
			Provider:  "stripe",
			EventID:   event.ID,
			EventType: event.Type,
		}, nil
	}
}

func (p *Provider) providerEventFromPrice(event stripeEvent, obj stripeObject, price Price, status domain.BillingStatus) *domain.ProviderWebhookEvent {
	return &domain.ProviderWebhookEvent{
		Provider:               "stripe",
		EventID:                event.ID,
		EventType:              event.Type,
		AccountID:              firstNonEmpty(metadataValue(obj.Metadata, "account_id"), metadataValue(obj.SubscriptionDetails.Metadata, "account_id")),
		ExternalCustomerID:     obj.Customer,
		ExternalSubscriptionID: firstNonEmpty(obj.ID, obj.Subscription, obj.Parent.SubscriptionDetails.Subscription),
		Plan:                   price.Plan,
		Status:                 status,
		ExternalPriceID:        price.StripeID,
		Currency:               price.Currency,
		AmountMinor:            price.AmountMinor,
		Interval:               price.Interval,
		CurrentPeriodEnd:       unixToRFC3339(obj.CurrentPeriodEnd),
		CancelAtPeriodEnd:      obj.CancelAtPeriodEnd,
	}
}

func (p *Provider) priceFromObject(obj stripeObject) Price {
	priceID := firstNonEmpty(obj.Items.Data.firstPriceID(), obj.Lines.Data.firstPriceID())
	price, ok := p.pricesByID[priceID]
	if !ok {
		return Price{StripeID: priceID}
	}
	if price.AmountMinor == 0 {
		price.AmountMinor = firstNonZero(obj.Items.Data.firstUnitAmount(), obj.Lines.Data.firstUnitAmount())
	}
	return price
}

func (p *Provider) postForm(ctx context.Context, path string, form url.Values, idempotencyKey string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.APIBase, "/")+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.cfg.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Stripe-Version", "2025-06-30.basil")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return stripeError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func (p *Provider) verifySignature(payload []byte, header string) error {
	timestamp, signatures := parseSignatureHeader(header)
	if timestamp == "" || len(signatures) == 0 {
		return domain.ErrBillingWebhookSignatureInvalid
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return domain.ErrBillingWebhookSignatureInvalid
	}
	signedAt := time.Unix(seconds, 0)
	if skew := p.now().Sub(signedAt); skew > 5*time.Minute || skew < -5*time.Minute {
		return domain.ErrBillingWebhookSignatureInvalid
	}
	mac := hmac.New(sha256.New, []byte(p.cfg.WebhookSecret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, sig := range signatures {
		actual, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(actual, expected) {
			return nil
		}
	}
	return domain.ErrBillingWebhookSignatureInvalid
}

func parseSignatureHeader(header string) (string, []string) {
	var timestamp string
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp = value
		case "v1":
			signatures = append(signatures, value)
		}
	}
	return timestamp, signatures
}

func stripeError(status int, body []byte) error {
	var res struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &res); err == nil && res.Error.Message != "" {
		return fmt.Errorf("stripe api error status=%d type=%s: %s", status, res.Error.Type, res.Error.Message)
	}
	return fmt.Errorf("stripe api error status=%d: %s", status, string(bytes.TrimSpace(body)))
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object stripeObject `json:"object"`
	} `json:"data"`
}

type stripeObject struct {
	ID                  string            `json:"id"`
	Customer            string            `json:"customer"`
	Subscription        string            `json:"subscription"`
	Status              string            `json:"status"`
	Metadata            map[string]string `json:"metadata"`
	CurrentPeriodEnd    int64             `json:"current_period_end"`
	CancelAtPeriodEnd   bool              `json:"cancel_at_period_end"`
	SubscriptionDetails struct {
		Metadata map[string]string `json:"metadata"`
	} `json:"subscription_details"`
	Items  stripeList `json:"items"`
	Lines  stripeList `json:"lines"`
	Parent struct {
		SubscriptionDetails struct {
			Subscription string `json:"subscription"`
		} `json:"subscription_details"`
	} `json:"parent"`
}

type stripeList struct {
	Data stripeLineItems `json:"data"`
}

type stripeLineItems []stripeLineItem

type stripeLineItem struct {
	Price stripePrice `json:"price"`
}

type stripePrice struct {
	ID         string `json:"id"`
	Currency   string `json:"currency"`
	UnitAmount int64  `json:"unit_amount"`
	Recurring  struct {
		Interval string `json:"interval"`
	} `json:"recurring"`
}

func (items stripeLineItems) firstPriceID() string {
	for _, item := range items {
		if item.Price.ID != "" {
			return item.Price.ID
		}
	}
	return ""
}

func (items stripeLineItems) firstUnitAmount() int64 {
	for _, item := range items {
		if item.Price.UnitAmount > 0 {
			return item.Price.UnitAmount
		}
	}
	return 0
}

func metadataValue(metadata map[string]string, key string) string {
	if metadata == nil {
		return ""
	}
	return metadata[key]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func unixToRFC3339(seconds int64) string {
	if seconds == 0 {
		return ""
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

func subscriptionStatus(status string) domain.BillingStatus {
	switch status {
	case "active", "trialing":
		return domain.BillingStatusActive
	case "past_due":
		return domain.BillingStatusPastDue
	case "unpaid":
		return domain.BillingStatusUnpaid
	case "canceled":
		return domain.BillingStatusCanceled
	case "incomplete", "incomplete_expired":
		return domain.BillingStatusIncomplete
	default:
		return ""
	}
}
