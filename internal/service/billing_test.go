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
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, "acc-1", "user-1", domain.BillingPlan("enterprise"), "")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBillingPlanInvalid)
	assert.Nil(t, session)
	assert.Zero(t, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.invalid_plan", logger.entries[0].event)
}

func TestCreateCheckoutSession_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, account.AccountID, "stranger", domain.BillingPlanUsageBased, "")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, session)
	assert.Zero(t, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.authorize_failed", logger.entries[0].event)
}

func TestCreateCheckoutSession_InvalidCurrency_WarnsAndReturnsError(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{}
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, "acc-1", "user-1", domain.BillingPlanUsageBased, domain.BillingCurrency("eur"))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrBillingCurrencyUnsupported)
	assert.Nil(t, session)
	assert.Zero(t, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.invalid_currency", logger.entries[0].event)
}

func TestCreateCheckoutSession_ProviderNotConfigured_LogsError(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)

	svc := NewBillingService(store, store, nil, logger)
	session, err := svc.CreateCheckoutSession(ctx, account.AccountID, "owner", domain.BillingPlanUsageBased, "")

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
		createCheckoutFn: func(ctx context.Context, gotAccount *domain.Account, gotPlan domain.BillingPlan, gotCurrency domain.BillingCurrency) (*domain.BillingCheckoutSession, error) {
			assert.Equal(t, account.AccountID, gotAccount.AccountID)
			assert.Equal(t, domain.BillingPlanUsageBased, gotPlan)
			assert.Empty(t, gotCurrency)
			return &domain.BillingCheckoutSession{URL: "https://checkout.example/session"}, nil
		},
	}
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreateCheckoutSession(ctx, account.AccountID, "owner", domain.BillingPlanUsageBased, "")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "https://checkout.example/session", session.URL)
	assert.Equal(t, 1, provider.createCheckoutCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "info", logger.entries[0].level)
	assert.Equal(t, "billing.checkout_session.created", logger.entries[0].event)
}

func TestCreatePortalSession_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreatePortalSession(ctx, account.AccountID, "stranger")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, session)
	assert.Zero(t, provider.createPortalCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.portal_session.authorize_failed", logger.entries[0].event)
}

func TestCreatePortalSession_Success_UsesAuthorizedAccount(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	provider := &billingTestProvider{
		createPortalFn: func(ctx context.Context, gotAccount *domain.Account) (*domain.BillingPortalSession, error) {
			assert.Equal(t, account.AccountID, gotAccount.AccountID)
			return &domain.BillingPortalSession{URL: "https://billing.example/portal"}, nil
		},
	}
	svc := NewBillingService(store, store, provider, logger)

	session, err := svc.CreatePortalSession(ctx, account.AccountID, "owner")

	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "https://billing.example/portal", session.URL)
	assert.Equal(t, 1, provider.createPortalCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "info", logger.entries[0].level)
	assert.Equal(t, "billing.portal_session.created", logger.entries[0].event)
}

func TestHandleWebhook_InvalidSignature_Warns(t *testing.T) {
	ctx := context.Background()
	logger := &billingTestLogger{}
	provider := &billingTestProvider{
		parseWebhookFn: func(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error) {
			return nil, domain.ErrBillingWebhookSignatureInvalid
		},
	}
	svc := NewBillingService(mock.NewStore(), mock.NewStore(), provider, logger)

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
	svc := NewBillingService(mock.NewStore(), mock.NewStore(), provider, logger)

	err := svc.HandleWebhook(ctx, []byte(`{}`), "sig")

	require.NoError(t, err)
	assert.Equal(t, 1, provider.parseWebhookCalls)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "info", logger.entries[0].level)
	assert.Equal(t, "billing.webhook.parsed", logger.entries[0].event)
}

// =========================================================
// Usage-Based Billing (stub) tests
// =========================================================

func TestGetUsage_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	report, err := svc.GetUsage(ctx, account.AccountID, "stranger", "", "")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, report)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.get_usage.authorize_failed", logger.entries[0].event)
}

func TestGetUsage_Success_ReturnsUsageReport(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	store.SeedPricing(domain.ModelPricing{
		Model:                    "gemini-3-flash-preview",
		InputCostPerMTokenMinor:  300,
		OutputCostPerMTokenMinor: 1500,
		Currency:                 "usd",
	})
	err = store.InsertUsageEvent(ctx, &domain.UsageEvent{
		EventID:      "evt-usage",
		AccountID:    account.AccountID,
		Model:        "gemini-3-flash-preview",
		InputTokens:  1_000_000,
		OutputTokens: 500_000,
		CostMinor:    1050,
		Currency:     "usd",
	})
	require.NoError(t, err)
	err = store.UpsertDailyUsage(ctx, account.AccountID, "2026-05-14", "gemini-3-flash-preview", 1_000_000, 500_000, 1050, 1)
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	report, err := svc.GetUsage(ctx, account.AccountID, "owner", "2026-05-01T00:00:00Z", "2026-05-31T23:59:59Z")

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Equal(t, account.AccountID, report.AccountID)
	assert.Equal(t, "2026-05-01T00:00:00Z", report.PeriodStart)
	assert.Equal(t, "2026-05-31T23:59:59Z", report.PeriodEnd)
	assert.Equal(t, "10.50", report.TotalCost)
	assert.Equal(t, "usd", report.Currency)
	require.Len(t, report.ByModel, 1)
	assert.Equal(t, "gemini-3-flash-preview", report.ByModel[0].Model)
	assert.Equal(t, "10.50", report.ByModel[0].TotalCost)
	require.Len(t, report.ByDay, 1)
	assert.Empty(t, logger.entries)
}

func TestRecordUsage_MissingFields_ReturnsUsageEventInvalid(t *testing.T) {
	// 目的: usage 記録に必要な冪等性キー・課金先・モデル名がない場合は永続化せず拒否する。
	// DB 層の制約エラーに落ちる前に service 境界で入力不備として扱うためのテスト。
	ctx := context.Background()
	logger := &billingTestLogger{}
	svc := NewBillingService(mock.NewStore(), mock.NewStore(), &billingTestProvider{}, logger)

	cases := []*domain.UsageEvent{
		nil,
		{Model: "gemini"},
		{AccountID: "acc-1"},
		{EventID: "evt-1", AccountID: "acc-1"},
		{AccountID: "acc-1", Model: "gemini"},
	}
	for _, ev := range cases {
		result, err := svc.RecordUsage(ctx, ev)
		require.Error(t, err)
		assert.ErrorIs(t, err, domain.ErrBillingUsageEventInvalid)
		assert.Nil(t, result)
	}
	assert.Empty(t, logger.entries)
}

func TestRecordUsage_ComputesCostFromPricing_PersistsEventAndRollup(t *testing.T) {
	// 目的: 価格表から token cost を計算し、raw event と日次 rollup に同じ event_id で保存する。
	// あわせて Stripe meter への best-effort 送信が通常経路で呼ばれることを確認する。
	ctx := context.Background()
	logger := &billingTestLogger{}
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	store.SeedPricing(domain.ModelPricing{
		Model:                    "gemini-3-flash-preview",
		InputCostPerMTokenMinor:  300,  // $3 / 1M input tokens
		OutputCostPerMTokenMinor: 1500, // $15 / 1M output tokens
		Currency:                 "usd",
	})
	provider := &billingTestProvider{}
	svc := NewBillingService(store, store, provider, logger)

	ev := &domain.UsageEvent{
		EventID:      "evt-1",
		AccountID:    account.AccountID,
		WorkspaceID:  "ws-1",
		JobID:        "job-1",
		Model:        "gemini-3-flash-preview",
		InputTokens:  1_000_000,
		OutputTokens: 500_000,
	}
	result, err := svc.RecordUsage(ctx, ev)

	require.NoError(t, err)
	require.NotNil(t, result)
	// 1M input * $3/M = $3.00, 500K output * $15/M = $7.50, total = $10.50
	assert.Equal(t, "10.50", result.Cost)
	assert.False(t, result.BudgetExceeded)
	assert.Equal(t, 1, provider.reportTokenUsageCalls, "should also push to Stripe meter")

	events := store.UsageEvents()
	require.Len(t, events, 1)
	assert.Equal(t, "evt-1", events[0].EventID)
	assert.Equal(t, int64(1050), events[0].CostMinor)
	assert.Equal(t, "usd", events[0].Currency)

	daily := store.DailyUsage()
	require.Len(t, daily, 1)
	for _, total := range daily {
		assert.Equal(t, int64(1050), total)
	}
}

func TestRecordUsage_UnknownModel_CostZeroButStillPersisted(t *testing.T) {
	// 目的: 未登録モデルでも forensic 用の raw event は捨てず、cost 0 として保存する。
	// pricing 未投入の新モデルで usage の欠損を起こさないことを守る。
	ctx := context.Background()
	logger := &billingTestLogger{}
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	ev := &domain.UsageEvent{
		EventID:      "evt-unknown",
		AccountID:    account.AccountID,
		Model:        "unmetered-model",
		InputTokens:  500,
		OutputTokens: 200,
	}
	result, err := svc.RecordUsage(ctx, ev)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "0.00", result.Cost)
	require.Len(t, store.UsageEvents(), 1, "event still persisted for forensics")
}

func TestRecordUsage_TogglesBudgetExceededOnFirstCross(t *testing.T) {
	// 目的: 累計 usage が budget を初めて超えた呼び出しだけ BudgetExceeded を返すことを確認する。
	// UI/通知が budget crossing を重複処理しないための境界テスト。
	ctx := context.Background()
	logger := &billingTestLogger{}
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	account.BudgetLimitMinor = 500 // $5 cap
	store.SeedPricing(domain.ModelPricing{
		Model:                    "test-model",
		InputCostPerMTokenMinor:  1_000_000, // $10000 / 1M token = 1 minor per token
		OutputCostPerMTokenMinor: 0,
		Currency:                 "usd",
	})
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	// First call: 300 tokens -> 300 minor. Under budget (500).
	first, err := svc.RecordUsage(ctx, &domain.UsageEvent{
		EventID: "evt-a", AccountID: account.AccountID, Model: "test-model", InputTokens: 300,
	})
	require.NoError(t, err)
	assert.False(t, first.BudgetExceeded)

	// Second call: 300 tokens -> total 600 minor, over budget. Should toggle on the first cross.
	second, err := svc.RecordUsage(ctx, &domain.UsageEvent{
		EventID: "evt-b", AccountID: account.AccountID, Model: "test-model", InputTokens: 300,
	})
	require.NoError(t, err)
	assert.True(t, second.BudgetExceeded, "budget should toggle exceeded on first cross")
}

func TestRecordUsage_NilUsageRepo_FallsBackToLoggingStub(t *testing.T) {
	// 目的: usage repository 未配線のローカル/移行中環境でも worker pipeline を止めないことを確認する。
	// この stub 経路でもログが出るため、無言で捨てられないことも見ている。
	ctx := context.Background()
	logger := &billingTestLogger{}
	svc := NewBillingService(mock.NewStore(), nil, &billingTestProvider{}, logger)

	result, err := svc.RecordUsage(ctx, &domain.UsageEvent{
		EventID: "evt-x", AccountID: "acc", Model: "any", InputTokens: 1, OutputTokens: 1,
	})

	require.NoError(t, err)
	assert.Equal(t, "0.00", result.Cost)
	// Stub path emits a single info log.
	require.NotEmpty(t, logger.entries)
}

func TestRecordUsage_StripeMeterFailureWarnsButKeepsAccounting(t *testing.T) {
	// 目的: Stripe meter 送信が失敗しても、内部 usage accounting は成功として返すことを確認する。
	// 外部課金 API の一時障害で worker job 全体を失敗させないためのテスト。
	ctx := context.Background()
	logger := &billingTestLogger{}
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	store.SeedPricing(domain.ModelPricing{
		Model:                    "test-model",
		InputCostPerMTokenMinor:  1_000_000,
		OutputCostPerMTokenMinor: 0,
		Currency:                 "usd",
	})
	provider := &billingTestProvider{
		reportTokenUsageFn: func(ctx context.Context, account *domain.Account, identifier string, inputTokens, outputTokens int64) error {
			return errors.New("stripe unavailable")
		},
	}
	svc := NewBillingService(store, store, provider, logger)

	result, err := svc.RecordUsage(ctx, &domain.UsageEvent{
		EventID: "evt-meter-failed", AccountID: account.AccountID, Model: "test-model", InputTokens: 10,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "0.10", result.Cost)
	require.Len(t, store.UsageEvents(), 1)
	require.NotEmpty(t, logger.entries)
	assert.Equal(t, "warn", logger.entries[len(logger.entries)-1].level)
	assert.Equal(t, "billing.record_usage.meter_event_failed", logger.entries[len(logger.entries)-1].event)
}

func TestUpdateBudget_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	limit, err := svc.UpdateBudget(ctx, account.AccountID, "stranger", "50.00")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, limit)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.update_budget.authorize_failed", logger.entries[0].event)
}

func TestUpdateBudget_Success_PersistsLimit(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	limit, err := svc.UpdateBudget(ctx, account.AccountID, "owner", "50.00")

	require.NoError(t, err)
	assert.Equal(t, "50.00", limit)
	updated, err := store.GetAccount(ctx, account.AccountID)
	require.NoError(t, err)
	assert.Equal(t, int64(5000), updated.BudgetLimitMinor)
	assert.Empty(t, logger.entries)
}

func TestListInvoices_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	list, err := svc.ListInvoices(ctx, account.AccountID, "stranger", 10)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, list)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.list_invoices.authorize_failed", logger.entries[0].event)
}

func TestListInvoices_Success_ReturnsEmptyList(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	list, err := svc.ListInvoices(ctx, account.AccountID, "owner", 10)

	require.NoError(t, err)
	require.NotNil(t, list)
	assert.Empty(t, list.Invoices)
	assert.Equal(t, "0.00", list.UpcomingAmount)
	assert.Empty(t, logger.entries)
}

func TestListPaymentMethods_OtherUser_DeniedAndReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	methods, err := svc.ListPaymentMethods(ctx, account.AccountID, "stranger")

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	assert.Nil(t, methods)
	require.Len(t, logger.entries, 1)
	assert.Equal(t, "warn", logger.entries[0].level)
	assert.Equal(t, "billing.list_payment_methods.authorize_failed", logger.entries[0].event)
}

func TestListPaymentMethods_Success_ReturnsEmptyList(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	logger := &billingTestLogger{}
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	svc := NewBillingService(store, store, &billingTestProvider{}, logger)

	methods, err := svc.ListPaymentMethods(ctx, account.AccountID, "owner")

	require.NoError(t, err)
	assert.Empty(t, methods)
	assert.Empty(t, logger.entries)
}

type billingTestProvider struct {
	createCheckoutCalls   int
	createPortalCalls     int
	parseWebhookCalls     int
	reportTokenUsageCalls int

	ensureCustomerFn   func(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error)
	createCheckoutFn   func(ctx context.Context, account *domain.Account, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error)
	createPortalFn     func(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error)
	parseWebhookFn     func(ctx context.Context, payload []byte, signature string) (*domain.ProviderWebhookEvent, error)
	reportTokenUsageFn func(ctx context.Context, account *domain.Account, identifier string, inputTokens, outputTokens int64) error
}

func (p *billingTestProvider) EnsureCustomer(ctx context.Context, account *domain.Account) (*domain.BillingCustomerRef, error) {
	if p.ensureCustomerFn == nil {
		return &domain.BillingCustomerRef{}, nil
	}
	return p.ensureCustomerFn(ctx, account)
}

func (p *billingTestProvider) CreateCheckoutSession(ctx context.Context, account *domain.Account, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error) {
	p.createCheckoutCalls++
	if p.createCheckoutFn == nil {
		return nil, errors.New("createCheckoutFn is not set")
	}
	return p.createCheckoutFn(ctx, account, plan, currency)
}

func (p *billingTestProvider) CreatePortalSession(ctx context.Context, account *domain.Account) (*domain.BillingPortalSession, error) {
	p.createPortalCalls++
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

func (p *billingTestProvider) ReportTokenUsage(ctx context.Context, account *domain.Account, identifier string, inputTokens, outputTokens int64) error {
	p.reportTokenUsageCalls++
	if p.reportTokenUsageFn == nil {
		return nil
	}
	return p.reportTokenUsageFn(ctx, account, identifier, inputTokens, outputTokens)
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
