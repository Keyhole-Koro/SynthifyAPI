package service

import (
	"context"
	"errors"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type BillingUsecase interface {
	CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error)
	CreatePortalSession(ctx context.Context, accountID, actorUserID string) (*domain.BillingPortalSession, error)
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

type BillingProvider interface {
	EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error)
	CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error)
	CreatePortalSession(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error)
	ParseWebhook(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error)
}

type billingService struct {
	accounts repository.AccountRepository
	provider BillingProvider
	logger   applog.Logger
}

func NewBillingService(accounts repository.AccountRepository, provider BillingProvider, logger applog.Logger) BillingUsecase {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &billingService{
		accounts: accounts,
		provider: provider,
		logger:   logger,
	}
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
