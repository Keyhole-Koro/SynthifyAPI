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
	CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan) (*domain.BillingCheckoutSession, error)
	CreatePortalSession(ctx context.Context, accountID, actorUserID string) (*domain.BillingPortalSession, error)
	HandleWebhook(ctx context.Context, payload []byte, signature string) error
}

type BillingProvider interface {
	EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error)
	CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan) (*domain.BillingCheckoutSession, error)
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

func (s *billingService) CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan) (*domain.BillingCheckoutSession, error) {
	if err := plan.Validate(); err != nil {
		s.logger.Warn(ctx, "billing.checkout_session.invalid_plan", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"plan":          plan,
		})
		return nil, err
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
	session, err := s.provider.CreateCheckoutSession(ctx, account, plan)
	if err != nil {
		s.noticeError(ctx, "billing.checkout_session.provider_failed", err, map[string]any{
			"account_id": accountID,
			"plan":       string(plan),
		})
		s.logger.Error(ctx, "billing.checkout_session.provider_failed", err, map[string]any{
			"account_id":    accountID,
			"actor_user_id": actorUserID,
			"plan":          plan,
		})
		return nil, err
	}
	s.logger.Info(ctx, "billing.checkout_session.created", map[string]any{
		"account_id":    accountID,
		"actor_user_id": actorUserID,
		"plan":          plan,
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
	_, err := s.provider.ParseWebhook(ctx, payload, signature)
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
	s.logger.Info(ctx, "billing.webhook.parsed", map[string]any{
		"payload_size": len(payload),
	})
	return err
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
	case errors.Is(err, domain.ErrBillingWebhookSignatureInvalid):
		return false
	case errors.Is(err, domain.ErrNotFound):
		return false
	default:
		return true
	}
}
