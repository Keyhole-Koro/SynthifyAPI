package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type BillingUsecase interface {
	GetBillingAccount(ctx context.Context, accountID, actorUserID string) (*domain.Account, error)
	CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error)
	CreatePortalSession(ctx context.Context, accountID, actorUserID string) (*domain.BillingPortalSession, error)
	HandleWebhook(ctx context.Context, payload []byte, signature string) error

	GetUsage(ctx context.Context, accountID, actorUserID string, periodStart, periodEnd string) (*domain.UsageReport, error)
	RecordUsage(ctx context.Context, ev *domain.UsageEvent) (*domain.UsageRecordResult, error)
	UpdateBudget(ctx context.Context, accountID, actorUserID string, budgetLimit string) (string, error)
	ListInvoices(ctx context.Context, accountID, actorUserID string, limit int) (*domain.InvoiceList, error)
	ListPaymentMethods(ctx context.Context, accountID, actorUserID string) ([]*domain.PaymentMethod, error)

	// Credits
	GrantFreeSignupCredit(ctx context.Context, accountID string) error
	GrantCredit(ctx context.Context, actorUserID, accountID string, amountMinor int64, note string) (*domain.CreditGrant, error)
	GetCreditBalance(ctx context.Context, accountID, actorUserID string) (int64, error)
}

type BillingProvider interface {
	EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error)
	CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error)
	CreatePortalSession(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error)
	ParseWebhook(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error)
	ReportTokenUsage(ctx context.Context, account *domain.Account, identifier string, inputTokens, outputTokens int64) error
}

type billingService struct {
	accounts repository.AccountRepository
	usage    repository.UsageRepository
	provider BillingProvider
	logger   applog.Logger
	now      func() time.Time
}

// NewBillingService wires the billing usecase. usage may be nil during early local dev
// (before the postgres-backed implementation lands); RecordUsage will then fall back
// to a logging-only stub so the worker pipeline keeps running.
func NewBillingService(accounts repository.AccountRepository, usage repository.UsageRepository, provider BillingProvider, logger applog.Logger) BillingUsecase {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &billingService{
		accounts: accounts,
		usage:    usage,
		provider: provider,
		logger:   logger,
		now:      time.Now,
	}
}

func (s *billingService) GetBillingAccount(ctx context.Context, accountID, actorUserID string) (*domain.Account, error) {
	account, err := s.authorizeAccount(ctx, accountID, actorUserID)
	if err != nil {
		s.logAuthorizeError(ctx, "billing.get_account.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	return account, nil
}

func (s *billingService) CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error) {
	if err := plan.Validate(); err != nil {
		s.logger.Warn(ctx, "billing.checkout_session.invalid_plan", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"plan":          plan,
		})
		return nil, err
	}
	if currency != "" {
		if err := currency.Validate(); err != nil {
			s.logger.Warn(ctx, "billing.checkout_session.invalid_currency", err, map[string]any{
				"account_id":    accountID,
				"actor_user_id": actorUserID,
				"currency":      currency,
			})
			return nil, err
		}
	}
	account, err := s.authorizeAccount(ctx, accountID, actorUserID)
	if err != nil {
		s.logAuthorizeError(ctx, "billing.checkout_session.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"plan":          plan,
		})
		return nil, err
	}
	if s.provider == nil {
		s.noticeError(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"account_id": accountID,
			"operation":  "create_checkout_session",
		})
		s.logger.Error(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"operation":     "create_checkout_session",
		})
		return nil, domain.ErrBillingProviderNotConfigured
	}
	if err := s.ensureProviderCustomer(ctx, account); err != nil {
		s.noticeError(ctx, "billing.checkout_session.customer_failed", err, map[string]any{
			"account_id": accountID,
		})
		s.logger.Error(ctx, "billing.checkout_session.customer_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	session, err := s.provider.CreateCheckoutSession(ctx, account, plan, currency)
	if err != nil {
		s.noticeError(ctx, "billing.checkout_session.provider_failed", err, map[string]any{
			"account_id": accountID,
			"plan":       string(plan),
			"currency":   string(currency),
		})
		s.logger.Error(ctx, "billing.checkout_session.provider_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"plan":          plan,
			"currency":      currency,
		})
		return nil, err
	}
	s.logger.Info(ctx, "billing.checkout_session.created", map[string]any{
		"account_id":    accountID,
		"actor_user_id": actorUserID,
		"plan":          plan,
		"currency":      currency,
	})
	return session, nil
}

func (s *billingService) CreatePortalSession(ctx context.Context, accountID, actorUserID string) (*domain.BillingPortalSession, error) {
	account, err := s.authorizeAccount(ctx, accountID, actorUserID)
	if err != nil {
		s.logAuthorizeError(ctx, "billing.portal_session.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	if s.provider == nil {
		s.noticeError(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"account_id": accountID,
			"operation":  "create_portal_session",
		})
		s.logger.Error(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"operation":     "create_portal_session",
		})
		return nil, domain.ErrBillingProviderNotConfigured
	}
	if err := s.ensureProviderCustomer(ctx, account); err != nil {
		s.noticeError(ctx, "billing.portal_session.customer_failed", err, map[string]any{
			"account_id": accountID,
		})
		s.logger.Error(ctx, "billing.portal_session.customer_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	session, err := s.provider.CreatePortalSession(ctx, account)
	if err != nil {
		s.noticeError(ctx, "billing.portal_session.provider_failed", err, map[string]any{
			"account_id": accountID,
		})
		s.logger.Error(ctx, "billing.portal_session.provider_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	s.logger.Info(ctx, "billing.portal_session.created", map[string]any{
		"account_id":    accountID,
		"actor_user_id": actorUserID,
	})
	return session, nil
}

func (s *billingService) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.provider == nil {
		s.noticeError(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"operation": "handle_webhook",
		})
		s.logger.Error(ctx, "billing.provider_not_configured", domain.ErrBillingProviderNotConfigured, map[string]any{
			"operation": "handle_webhook",
		})
		return domain.ErrBillingProviderNotConfigured
	}
	event, err := s.provider.ParseWebhook(ctx, payload, signature)
	if err != nil {
		if errors.Is(err, domain.ErrBillingWebhookSignatureInvalid) {
			s.logger.Warn(ctx, "billing.webhook.invalid_signature", err, map[string]any{
				"payload_size": len(payload),
			})
			return err
		}
		s.noticeError(ctx, "billing.webhook.parse_failed", err, map[string]any{
			"payload_size": len(payload),
		})
		s.logger.Error(ctx, "billing.webhook.parse_failed", err, map[string]any{
			"payload_size": len(payload),
		})
		return err
	}
	applied, err := s.recordAndApplyWebhookEvent(ctx, event)
	if err != nil {
		s.noticeError(ctx, "billing.webhook.apply_failed", err, map[string]any{
			"event_id":   event.EventID,
			"event_type": event.EventType,
		})
		s.logger.Error(ctx, "billing.webhook.apply_failed", err, map[string]any{
			"event_id":                 event.EventID,
			"event_type":               event.EventType,
			"account_id":               event.AccountID,
			"external_customer_id":     event.ExternalCustomerID,
			"external_subscription_id": event.ExternalSubscriptionID,
		})
		return err
	}
	s.logger.Info(ctx, "billing.webhook.parsed", map[string]any{
		"payload_size":             len(payload),
		"event_id":                 event.EventID,
		"event_type":               event.EventType,
		"account_id":               event.AccountID,
		"external_customer_id":     event.ExternalCustomerID,
		"external_subscription_id": event.ExternalSubscriptionID,
		"applied":                  applied,
	})
	return err
}

func (s *billingService) ensureProviderCustomer(ctx context.Context, account *domain.Account) error {
	customer, err := s.provider.EnsureCustomer(ctx, account)
	if err != nil {
		return err
	}
	if customer == nil || customer.ExternalCustomerID == "" || customer.ExternalCustomerID == account.StripeCustomerID {
		return nil
	}
	if err := s.accounts.SetAccountStripeCustomerID(ctx, account.AccountID, customer.ExternalCustomerID); err != nil {
		return err
	}
	account.StripeCustomerID = customer.ExternalCustomerID
	return nil
}

func (s *billingService) recordAndApplyWebhookEvent(ctx context.Context, event *domain.ProviderWebhookEvent) (bool, error) {
	if event == nil {
		return false, nil
	}
	recorded, err := s.accounts.RecordBillingWebhookEvent(ctx, event)
	if err != nil {
		return false, err
	}
	if !recorded {
		s.logger.Info(ctx, "billing.webhook.duplicate", map[string]any{
			"event_id":   event.EventID,
			"event_type": event.EventType,
		})
		return false, nil
	}
	if event.Plan == "" {
		if err := s.accounts.MarkBillingWebhookEventProcessed(ctx, event.Provider, event.EventID, "ignored", ""); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := event.Plan.Validate(); err != nil {
		_ = s.accounts.MarkBillingWebhookEventProcessed(ctx, event.Provider, event.EventID, "failed", err.Error())
		return false, err
	}
	if event.Currency != "" {
		if err := event.Currency.Validate(); err != nil {
			_ = s.accounts.MarkBillingWebhookEventProcessed(ctx, event.Provider, event.EventID, "failed", err.Error())
			return false, err
		}
	}
	if err := s.accounts.ApplyBillingEvent(ctx, event); err != nil {
		_ = s.accounts.MarkBillingWebhookEventProcessed(ctx, event.Provider, event.EventID, "failed", err.Error())
		return false, err
	}
	if err := s.accounts.MarkBillingWebhookEventProcessed(ctx, event.Provider, event.EventID, "processed", ""); err != nil {
		return true, err
	}
	return true, nil
}

func (s *billingService) authorizeAccount(ctx context.Context, accountID, userID string) (*domain.Account, error) {
	if !s.accounts.IsAccountAccessible(ctx, accountID, userID) {
		return nil, domain.ErrNotFound
	}
	return s.accounts.GetAccount(ctx, accountID)
}

func (s *billingService) noticeError(ctx context.Context, event string, err error, attrs map[string]any) {
	if !shouldNoticeBillingError(err) {
		return
	}
	txn := newrelic.FromContext(ctx)
	if txn == nil || err == nil {
		return
	}
	txn.AddAttribute("event", event)
	for key, value := range attrs {
		txn.AddAttribute(key, value)
	}
	txn.NoticeError(err)
}

func (s *billingService) logAuthorizeError(ctx context.Context, event string, err error, fields map[string]any) {
	if errors.Is(err, domain.ErrNotFound) {
		s.logger.Warn(ctx, event, err, fields)
		return
	}
	s.logger.Error(ctx, event, err, fields)
}

func shouldNoticeBillingError(err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, domain.ErrBillingPlanInvalid):
		return false
	case errors.Is(err, domain.ErrBillingCurrencyUnsupported):
		return false
	case errors.Is(err, domain.ErrBillingWebhookSignatureInvalid):
		return false
	case errors.Is(err, domain.ErrNotFound):
		return false
	default:
		return true
	}
}

// =========================================================
// Usage-Based Billing
// 仕様: docs/architecture/usage-based-billing-spec.md
// =========================================================

func (s *billingService) GetUsage(ctx context.Context, accountID, actorUserID string, periodStart, periodEnd string) (*domain.UsageReport, error) {
	if _, err := s.authorizeAccount(ctx, accountID, actorUserID); err != nil {
		s.logAuthorizeError(ctx, "billing.get_usage.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	if s.usage == nil {
		return &domain.UsageReport{
			AccountID:   accountID,
			PeriodStart: periodStart,
			PeriodEnd:   periodEnd,
			TotalCost:   "0.00",
			Currency:    "usd",
		}, nil
	}
	byModel, currency, err := s.usage.ListUsageByModel(ctx, accountID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	byDay, err := s.usage.ListDailyUsage(ctx, accountID, periodStart, periodEnd, currency)
	if err != nil {
		return nil, err
	}
	totalMinor := int64(0)
	for _, row := range byModel {
		minor, err := parseMinor(row.TotalCost, currency)
		if err != nil {
			continue
		}
		totalMinor += minor
	}
	return &domain.UsageReport{
		AccountID:   accountID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		TotalCost:   formatMinor(totalMinor, currency),
		Currency:    currency,
		ByModel:     byModel,
		ByDay:       byDay,
	}, nil
}

func (s *billingService) RecordUsage(ctx context.Context, ev *domain.UsageEvent) (*domain.UsageRecordResult, error) {
	if ev == nil || ev.EventID == "" || ev.AccountID == "" || ev.Model == "" {
		return nil, domain.ErrBillingUsageEventInvalid
	}
	// 認可は handler 側で middleware.IsServiceCall を要求済み (X-Synthify-Service-Token).

	// When the usage repository is not wired (early dev), keep the legacy logging stub
	// so the worker pipeline still flows; still attempt to push to Stripe meter.
	if s.usage == nil {
		s.logger.Info(ctx, "billing.record_usage.stub", map[string]any{
			"account_id":    ev.AccountID,
			"workspace_id":  ev.WorkspaceID,
			"job_id":        ev.JobID,
			"model":         ev.Model,
			"input_tokens":  ev.InputTokens,
			"output_tokens": ev.OutputTokens,
		})
		s.reportStripeMeterPortion(ctx, ev, ev.InputTokens, ev.OutputTokens)
		return &domain.UsageRecordResult{EventID: ev.EventID, Cost: "0.00"}, nil
	}

	// 1. Pricing lookup. Unknown model -> cost 0 + warn but keep persisting for forensics.
	currency := "usd"
	costMinor := int64(0)
	pricing, err := s.usage.GetModelPricing(ctx, ev.Model)
	switch {
	case err == nil && pricing != nil:
		costMinor = computeCostMinor(pricing, ev.InputTokens, ev.OutputTokens)
		if pricing.Currency != "" {
			currency = pricing.Currency
		}
	case errors.Is(err, domain.ErrNotFound):
		s.logger.Warn(ctx, "billing.record_usage.no_pricing", nil, map[string]any{
			"model":      ev.Model,
			"account_id": ev.AccountID,
		})
	default:
		s.logger.Error(ctx, "billing.record_usage.pricing_lookup_failed", err, map[string]any{"model": ev.Model})
	}

	ev.CostMinor = costMinor
	ev.Currency = currency

	// 2. 厳密版: クレジット残高があれば優先消費し、不足分は Stripe usage-based meter に流す。
	//    - balance >= cost  : 全額 credit (paid_via=credit)
	//    - 0 < balance < cost && usage_based : credit 全消費 + 超過分を Stripe (paid_via=mixed)
	//    - balance <= 0   && usage_based : 全額 Stripe (paid_via=stripe)
	//    - balance <= 0   && free        : 停止 (CreditStopped=true)
	creditStopped := false
	creditPortion := int64(0)
	stripePortion := int64(0)
	paidVia := domain.PaidViaStripe

	if costMinor > 0 {
		balance, balErr := s.usage.GetCreditBalance(ctx, ev.AccountID)
		if balErr != nil {
			s.logger.Warn(ctx, "billing.record_usage.balance_lookup_failed", balErr, map[string]any{"account_id": ev.AccountID})
		}
		account, accErr := s.accounts.GetAccount(ctx, ev.AccountID)
		isFree := accErr == nil && account != nil && account.Plan == string(domain.BillingPlanFree)

		switch {
		case balance >= costMinor:
			creditPortion = costMinor
			paidVia = domain.PaidViaCredit
		case balance > 0:
			creditPortion = balance
			stripePortion = costMinor - balance
			paidVia = domain.PaidViaMixed
			if isFree {
				// usage_based 未登録なのに超過した → Stripe に送れないので停止扱い。
				creditStopped = true
			}
		case isFree:
			// 残高なし & free plan → 停止。Stripe meter にも送らない。
			creditStopped = true
			paidVia = domain.PaidViaCredit // 課金経路なし
		default:
			stripePortion = costMinor
			paidVia = domain.PaidViaStripe
		}

		// クレジット消費分を負の grant として記録 (best-effort).
		if creditPortion > 0 {
			deduct := &domain.CreditGrant{
				CreditID:    "deduct-" + ev.EventID,
				AccountID:   ev.AccountID,
				CreditType:  domain.CreditTypeConsumed,
				AmountMinor: -creditPortion,
				Currency:    currency,
				Note:        "usage:" + ev.EventID,
				GrantedBy:   "system",
				GrantedAt:   s.now().UTC().Format("2006-01-02T15:04:05Z"),
			}
			if err := s.usage.GrantCredit(ctx, deduct); err != nil {
				s.logger.Warn(ctx, "billing.record_usage.credit_deduct_failed", err, map[string]any{
					"event_id":   ev.EventID,
					"account_id": ev.AccountID,
				})
			}
		}
		if creditStopped {
			s.logger.Info(ctx, "billing.record_usage.credit_exhausted", map[string]any{
				"account_id": ev.AccountID,
				"event_id":   ev.EventID,
				"balance":    balance,
				"cost":       costMinor,
			})
		}
	}

	ev.PaidVia = paidVia
	ev.CreditAmountMinor = creditPortion
	ev.StripeAmountMinor = stripePortion

	// 3. Persist raw event, daily rollup, and account accumulator atomically.
	date := s.now().UTC().Format("2006-01-02")
	_, exceeded, err := s.usage.RecordUsageAccounting(ctx, ev, date)
	if err != nil {
		s.logger.Error(ctx, "billing.record_usage.accounting_failed", err, map[string]any{"event_id": ev.EventID, "account_id": ev.AccountID})
		return nil, err
	}

	// 4. Stripe meter event は Stripe portion がある場合のみ送る。
	//    token 数は cost-比例で按分する (端数は input 側に寄せる)。
	if stripePortion > 0 {
		inTok, outTok := proratedTokens(ev.InputTokens, ev.OutputTokens, costMinor, stripePortion)
		s.reportStripeMeterPortion(ctx, ev, inTok, outTok)
	}

	return &domain.UsageRecordResult{
		EventID:           ev.EventID,
		Cost:              formatMinor(costMinor, currency),
		BudgetExceeded:    exceeded || creditStopped,
		CreditStopped:     creditStopped,
		PaidVia:           paidVia,
		CreditAmountMinor: creditPortion,
		StripeAmountMinor: stripePortion,
	}, nil
}

// proratedTokens splits (inputTokens, outputTokens) by the ratio stripePortion/totalCost,
// rounding the input side up so that small mixed events still report at least 1 input token.
func proratedTokens(inputTokens, outputTokens, totalCost, stripePortion int64) (int64, int64) {
	if totalCost <= 0 || stripePortion >= totalCost {
		return inputTokens, outputTokens
	}
	stripeIn := (inputTokens*stripePortion + totalCost - 1) / totalCost
	stripeOut := (outputTokens * stripePortion) / totalCost
	if stripeIn > inputTokens {
		stripeIn = inputTokens
	}
	if stripeOut > outputTokens {
		stripeOut = outputTokens
	}
	return stripeIn, stripeOut
}

func (s *billingService) reportStripeMeterPortion(ctx context.Context, ev *domain.UsageEvent, inputTokens, outputTokens int64) {
	if s.provider == nil {
		return
	}
	account, err := s.accounts.GetAccount(ctx, ev.AccountID)
	if err != nil || account == nil {
		return
	}
	if err := s.provider.ReportTokenUsage(ctx, account, ev.EventID, inputTokens, outputTokens); err != nil {
		s.logger.Warn(ctx, "billing.record_usage.meter_event_failed", err, map[string]any{
			"account_id": ev.AccountID,
			"event_id":   ev.EventID,
		})
	}
}

// computeCostMinor: cost_minor = (tokens * rate_per_mtoken_minor) / 1_000_000.
// Integer truncation is intentional — fractional minor units cannot be billed anyway.
func computeCostMinor(p *domain.ModelPricing, inputTokens, outputTokens int64) int64 {
	const million = int64(1_000_000)
	return (inputTokens*p.InputCostPerMTokenMinor)/million + (outputTokens*p.OutputCostPerMTokenMinor)/million
}

// formatMinor renders a minor-unit amount as a decimal string in the conventional
// presentation for the currency (2 decimal places for cents currencies, integer for JPY).
func formatMinor(minor int64, currency string) string {
	if currency == "jpy" {
		return strconv.FormatInt(minor, 10)
	}
	neg := ""
	if minor < 0 {
		neg = "-"
		minor = -minor
	}
	return neg + strconv.FormatInt(minor/100, 10) + "." + fmt.Sprintf("%02d", minor%100)
}

func parseMinor(value string, currency string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if currency == "jpy" {
		return strconv.ParseInt(value, 10, 64)
	}
	neg := false
	if strings.HasPrefix(value, "-") {
		neg = true
		value = strings.TrimPrefix(value, "-")
	}
	whole, frac, ok := strings.Cut(value, ".")
	if !ok {
		frac = ""
	}
	if whole == "" {
		whole = "0"
	}
	if len(frac) > 2 {
		return 0, domain.ErrBillingBudgetInvalid
	}
	for len(frac) < 2 {
		frac += "0"
	}
	wholeMinor, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	fracMinor := int64(0)
	if frac != "" {
		fracMinor, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	minor := wholeMinor*100 + fracMinor
	if neg {
		minor = -minor
	}
	return minor, nil
}

func (s *billingService) UpdateBudget(ctx context.Context, accountID, actorUserID string, budgetLimit string) (string, error) {
	account, err := s.authorizeAccount(ctx, accountID, actorUserID)
	if err != nil {
		s.logAuthorizeError(ctx, "billing.update_budget.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return "", err
	}
	currency := account.BillingCurrency
	if currency == "" {
		currency = "usd"
	}
	limitMinor, err := parseMinor(budgetLimit, currency)
	if err != nil || limitMinor < 0 {
		return "", domain.ErrBillingBudgetInvalid
	}
	if s.usage == nil {
		return formatMinor(limitMinor, currency), nil
	}
	if err := s.usage.UpdateAccountBudgetLimit(ctx, accountID, limitMinor); err != nil {
		return "", err
	}
	return formatMinor(limitMinor, currency), nil
}

func (s *billingService) ListInvoices(ctx context.Context, accountID, actorUserID string, limit int) (*domain.InvoiceList, error) {
	if _, err := s.authorizeAccount(ctx, accountID, actorUserID); err != nil {
		s.logAuthorizeError(ctx, "billing.list_invoices.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	if s.usage == nil {
		return &domain.InvoiceList{Invoices: nil, UpcomingAmount: "0.00", UpcomingPeriodEnd: ""}, nil
	}
	return s.usage.ListInvoices(ctx, accountID, limit)
}

func (s *billingService) ListPaymentMethods(ctx context.Context, accountID, actorUserID string) ([]*domain.PaymentMethod, error) {
	if _, err := s.authorizeAccount(ctx, accountID, actorUserID); err != nil {
		s.logAuthorizeError(ctx, "billing.list_payment_methods.authorize_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
		})
		return nil, err
	}
	if s.usage == nil {
		return nil, nil
	}
	return s.usage.ListPaymentMethods(ctx, accountID)
}

// =========================================================
// Credits
// =========================================================

// GrantFreeSignupCredit は新規アカウント作成時に $1 の無料クレジットを付与する。
// 既に付与済みの場合は冪等に何もしない（credit_id が衝突するため）。
func (s *billingService) GrantFreeSignupCredit(ctx context.Context, accountID string) error {
	if s.usage == nil {
		return nil
	}
	grant := &domain.CreditGrant{
		CreditID:    "free-signup-" + accountID,
		AccountID:   accountID,
		CreditType:  domain.CreditTypeFree,
		AmountMinor: domain.FreeSignupCreditMinor,
		Currency:    "usd",
		Note:        "signup bonus",
		GrantedBy:   "system",
		GrantedAt:   s.now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if err := s.usage.GrantCredit(ctx, grant); err != nil {
		s.logger.Warn(ctx, "billing.grant_free_signup_credit.failed", err, map[string]any{
			"account_id": accountID,
		})
		return err
	}
	s.logger.Info(ctx, "billing.grant_free_signup_credit.ok", map[string]any{
		"account_id":   accountID,
		"amount_minor": domain.FreeSignupCreditMinor,
	})
	return nil
}

// GrantCredit は admin が任意アカウントにクレジットを付与する。
func (s *billingService) GrantCredit(ctx context.Context, actorUserID, accountID string, amountMinor int64, note string) (*domain.CreditGrant, error) {
	if amountMinor <= 0 {
		return nil, domain.ErrBillingBudgetInvalid
	}
	if s.usage == nil {
		return nil, domain.ErrBillingProviderNotConfigured
	}
	grant := &domain.CreditGrant{
		CreditID:    newCreditID(),
		AccountID:   accountID,
		CreditType:  domain.CreditTypeFree,
		AmountMinor: amountMinor,
		Currency:    "usd",
		Note:        note,
		GrantedBy:   actorUserID,
		GrantedAt:   s.now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if err := s.usage.GrantCredit(ctx, grant); err != nil {
		return nil, err
	}
	return grant, nil
}

func (s *billingService) GetCreditBalance(ctx context.Context, accountID, actorUserID string) (int64, error) {
	if _, err := s.authorizeAccount(ctx, accountID, actorUserID); err != nil {
		return 0, err
	}
	if s.usage == nil {
		return 0, nil
	}
	return s.usage.GetCreditBalance(ctx, accountID)
}

func newCreditID() string {
	return fmt.Sprintf("credit-%d", time.Now().UnixNano())
}
