package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func TestCreateCheckoutSession_InvalidPlan_WarnsAndReturnsError(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{}
	svc := NewBillingService(store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, "acc-1", "user-1", domain.BillingPlan("enterprise"))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBillingPlanInvalid)
	assert.Nil(t, session)
	assert.Zero(t, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.invalid_plan", logger.entries[0].event)
}

func TestCreateCheckoutSession_AccountNotAccessible_WarnsAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{}
	svc := NewBillingService(store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, "owner", "stranger", domain.BillingPlanPro)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, session)
	assert.Zero(t, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.authorize_failed", logger.entries[0].event)
}

func TestCreateCheckoutSession_ProviderNotConfigured_LogsError(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)

	svc := NewBillingService(store, nil, logger)
	session, err := svc.CreateCheckoutSession(ctx, account.AccountID, "owner", domain.BillingPlanPro)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBillingProviderNotConfigured)
	assert.Nil(t, session)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "error", logger.entries[0].level)
	assert.Equal(t, "billing.provider_not_configured", logger.entries[0].event)
}

func TestCreateCheckoutSession_Success_LogsInfo(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	provider := &billingTestProvider{
		createCheckoutFn: func(ctx context.Context, gotAccount *domain.Account, gotPlan domain.BillingPlan) (*domain.BillingCheckoutSession, error) {
			assert.Equal(t, account.AccountID, gotAccount.AccountID)
			assert.Equal(t, domain.BillingPlanPro, gotPlan)
			return &domain.BillingCheckoutSession{URL: "https://checkout.example/session"}, nil
		},
	}
	svc := NewBillingService(store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, account.AccountID, "owner", domain.BillingPlanPro)

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "https://checkout.example/session", session.URL)
	assert.Equal(t, 1, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "info", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.created", logger.entries[0].event)
}

func TestHandleWebhook_InvalidSignature_Warns(t *testing.T) {
	ctx := context.Background()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{
		parseWebhookFn: func(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error) {
			return nil, domain.ErrBillingWebhookSignatureInvalid
		},
	}
	svc := NewBillingService(mock.NewStore(), provider, logger)

	err := svc.HandleWebhook(ctx, []byte(`{}`), "sig")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBillingWebhookSignatureInvalid)
	assert.Equal(t, 1, provider.parseWebhookCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.webhook.invalid_signature", logger.entries[0].event)
}

func TestHandleWebhook_Success_LogsInfo(t *testing.T) {
	ctx := context.Background()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{
		parseWebhookFn: func(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error) {
			return &domain.ProviderWebhookEvent{EventID: "evt_1", EventType: "checkout.session.completed"}, nil
		},
	}
	svc := NewBillingService(mock.NewStore(), provider, logger)

	err := svc.HandleWebhook(ctx, []byte(`{}`), "sig")

	require.NoError(t, err)
	assert.Equal(t, 1, provider.parseWebhookCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "info", logger.entries[0].level)
	assert.Equal(t, "billing.webhook.parsed", logger.entries[0].event)
}

type billingTestProvider struct {
	createCheckoutCalls int
	parseWebhookCalls   int

	ensureCustomerFn  func(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error)
	createCheckoutFn  func(ctx context.Context, account *domain.Account, plan domain.BillingPlan) (*domain.BillingCheckoutSession, error)
	createPortalFn    func(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error)
	parseWebhookFn    func(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error)
}

func (p *billingTestProvider) EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error) {
	if p.ensureCustomerFn == nil {
		return &domain.BillingCustomerRef{}, nil
	}
	return p.ensureCustomerFn(ctx, account)
}

func (p *billingTestProvider) CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan) (*domain.BillingCheckoutSession, error) {
	p.createCheckoutCalls++
	if p.createCheckoutFn == nil {
		return nil, errors.New("createCheckoutFn is not set")
	}
	return p.createCheckoutFn(ctx, account, plan)
}

func (p *billingTestProvider) CreatePortalSession(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error) {
	if p.createPortalFn == nil {
		return &domain.BillingPortalSession{}, nil
	}
	return p.createPortalFn(ctx, account)
}

func (p *billingTestProvider) ParseWebhook(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error) {
	p.parseWebhookCalls++
	if p.parseWebhookFn == nil {
		return nil, errors.New("parseWebhookFn is not set")
	}
	return p.parseWebhookFn(ctx, payload, signature)
}

type billingTestLogger struct {
	entries []billingLogEntry
}

type billingLogEntry struct {
	level  string
	event  string
	err    error
	fields map[string]any
}

func (l *billingTestLogger) Info(ctx context.Context, event string, fields map[string]any) {
	l.entries = append(l.entries, billingLogEntry{
		level:  "info",
		event:  event,
		fields: cloneFields(fields),
	})
}

func (l *billingTestLogger) Warn(ctx context.Context, event string, err error, fields map[string]any) {
	l.entries = append(l.entries, billingLogEntry{
		level:  "warn",
		event:  event,
		err:    err,
		fields: cloneFields(fields),
	})
}

func (l *billingTestLogger) Error(ctx context.Context, event string, err error, fields map[string]any) {
	l.entries = append(l.entries, billingLogEntry{
		level:  "error",
		event:  event,
		err:    err,
		fields: cloneFields(fields),
	})
}

func cloneFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}
