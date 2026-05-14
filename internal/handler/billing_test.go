package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/middleware"
)

func TestBillingHandler_CreateCheckoutSession_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)

	resp, err := h.CreateCheckoutSession(context.Background(), connect.NewRequest(&treev1.CreateCheckoutSessionRequest{
		AccountId: "owner",
		Currency:  string(domain.BillingCurrencyUSD),
	}))

	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.createCheckoutCalls)
}

func TestBillingHandler_CreateCheckoutSession_UsesAuthenticatedUser(t *testing.T) {
	svc := &billingHandlerTestUsecase{
		createCheckoutSession: &domain.BillingCheckoutSession{URL: "https://checkout.example/session"},
	}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

	resp, err := h.CreateCheckoutSession(ctx, connect.NewRequest(&treev1.CreateCheckoutSessionRequest{
		AccountId: "owner",
		Currency:  string(domain.BillingCurrencyUSD),
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "https://checkout.example/session", resp.Msg.GetCheckoutUrl())
	assert.Equal(t, 1, svc.createCheckoutCalls)
	assert.Equal(t, "owner", svc.gotCheckoutAccountID)
	assert.Equal(t, "owner", svc.gotCheckoutActorUserID)
	assert.Equal(t, domain.BillingPlanUsageBased, svc.gotCheckoutPlan)
	assert.Equal(t, domain.BillingCurrencyUSD, svc.gotCheckoutCurrency)
}

func TestBillingHandler_CreatePortalSession_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)

	resp, err := h.CreatePortalSession(context.Background(), connect.NewRequest(&treev1.CreatePortalSessionRequest{
		AccountId: "owner",
	}))

	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.createPortalCalls)
}

func TestBillingHandler_CreatePortalSession_UsesAuthenticatedUser(t *testing.T) {
	svc := &billingHandlerTestUsecase{
		createPortalSession: &domain.BillingPortalSession{URL: "https://billing.example/portal"},
	}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

	resp, err := h.CreatePortalSession(ctx, connect.NewRequest(&treev1.CreatePortalSessionRequest{
		AccountId: "owner",
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "https://billing.example/portal", resp.Msg.GetPortalUrl())
	assert.Equal(t, 1, svc.createPortalCalls)
	assert.Equal(t, "owner", svc.gotPortalAccountID)
	assert.Equal(t, "owner", svc.gotPortalActorUserID)
}

// =========================================================
// Authentication required for every billing RPC
// =========================================================

func TestBillingHandler_GetBillingAccount_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	resp, err := h.GetBillingAccount(context.Background(), connect.NewRequest(&treev1.GetBillingAccountRequest{AccountId: "owner"}))
	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestBillingHandler_GetUsage_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	resp, err := h.GetUsage(context.Background(), connect.NewRequest(&treev1.GetUsageRequest{AccountId: "owner"}))
	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.getUsageCalls)
}

func TestBillingHandler_GetUsage_AuthenticatedUser_CallsService(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	resp, err := h.GetUsage(ctx, connect.NewRequest(&treev1.GetUsageRequest{AccountId: "owner"}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, svc.getUsageCalls)
}

func TestBillingHandler_UpdateBudget_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	resp, err := h.UpdateBudget(context.Background(), connect.NewRequest(&treev1.UpdateBudgetRequest{AccountId: "owner", BudgetLimit: "50"}))
	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.updateBudgetCalls)
}

func TestBillingHandler_UpdateBudget_AuthenticatedUser_CallsService(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	resp, err := h.UpdateBudget(ctx, connect.NewRequest(&treev1.UpdateBudgetRequest{AccountId: "owner", BudgetLimit: "50"}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, svc.updateBudgetCalls)
}

func TestBillingHandler_ListInvoices_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	resp, err := h.ListInvoices(context.Background(), connect.NewRequest(&treev1.ListInvoicesRequest{AccountId: "owner"}))
	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.listInvoicesCalls)
}

func TestBillingHandler_ListInvoices_AuthenticatedUser_CallsService(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	resp, err := h.ListInvoices(ctx, connect.NewRequest(&treev1.ListInvoicesRequest{AccountId: "owner", Limit: 10}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, svc.listInvoicesCalls)
}

func TestBillingHandler_ListPaymentMethods_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	resp, err := h.ListPaymentMethods(context.Background(), connect.NewRequest(&treev1.ListPaymentMethodsRequest{AccountId: "owner"}))
	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
	assert.Zero(t, svc.listPaymentMethodsCalls)
}

func TestBillingHandler_ListPaymentMethods_AuthenticatedUser_CallsService(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	resp, err := h.ListPaymentMethods(ctx, connect.NewRequest(&treev1.ListPaymentMethodsRequest{AccountId: "owner"}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, svc.listPaymentMethodsCalls)
}

// RecordUsage は worker -> API の内部 RPC。
// middleware が service token を検証して ContextWithServiceCall を立てた場合のみ通る。
func TestBillingHandler_RecordUsage_NoServiceToken_ReturnsPermissionDenied(t *testing.T) {
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)

	resp, err := h.RecordUsage(context.Background(), connect.NewRequest(&treev1.RecordUsageRequest{
		AccountId: "owner",
		Model:     "gemini-2.5-pro",
	}))

	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodePermissionDenied)
	assert.Zero(t, svc.recordUsageCalls)
}

func TestBillingHandler_RecordUsage_AuthenticatedUserAlone_StillDenied(t *testing.T) {
	// 通常ユーザーの認証だけでは RecordUsage は呼べない。
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	resp, err := h.RecordUsage(ctx, connect.NewRequest(&treev1.RecordUsageRequest{
		AccountId: "owner",
		Model:     "gemini-2.5-pro",
	}))

	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodePermissionDenied)
	assert.Zero(t, svc.recordUsageCalls)
}

func TestBillingHandler_RecordUsage_ServiceToken_CallsService(t *testing.T) {
	// 目的: worker が送った usage payload が欠落なく service 層へ渡ることを確認する。
	// 特に event_id は DB の冪等性キーなので、handler 境界で落とさないことを守る。
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithServiceCall(context.Background())

	resp, err := h.RecordUsage(ctx, connect.NewRequest(&treev1.RecordUsageRequest{
		EventId:      "evt-usage-1",
		AccountId:    "owner",
		WorkspaceId:  "ws-1",
		JobId:        "job-1",
		Model:        "gemini-2.5-pro",
		InputTokens:  123,
		OutputTokens: 45,
	}))

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, svc.recordUsageCalls)
	require.NotNil(t, svc.gotUsageEvent)
	assert.Equal(t, "evt-usage-1", svc.gotUsageEvent.EventID)
	assert.Equal(t, "owner", svc.gotUsageEvent.AccountID)
	assert.Equal(t, "ws-1", svc.gotUsageEvent.WorkspaceID)
	assert.Equal(t, "job-1", svc.gotUsageEvent.JobID)
	assert.Equal(t, "gemini-2.5-pro", svc.gotUsageEvent.Model)
	assert.Equal(t, int64(123), svc.gotUsageEvent.InputTokens)
	assert.Equal(t, int64(45), svc.gotUsageEvent.OutputTokens)
	assert.Equal(t, "evt-usage-1", resp.Msg.GetEventId())
}

func TestBillingHandler_RecordUsage_ServiceTokenMissingEventID_ReturnsInvalidArgument(t *testing.T) {
	// 目的: service token が正しくても、冪等性キー event_id がない usage 記録は拒否することを確認する。
	// これにより DB 永続化直前で失敗するのではなく、handler 境界で入力不備として扱える。
	svc := &billingHandlerTestUsecase{}
	h := NewBillingHandler(svc)
	ctx := middleware.ContextWithServiceCall(context.Background())

	resp, err := h.RecordUsage(ctx, connect.NewRequest(&treev1.RecordUsageRequest{
		AccountId: "owner",
		Model:     "gemini-2.5-pro",
	}))

	assert.Nil(t, resp)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
	assert.Zero(t, svc.recordUsageCalls)
}

func TestBillingWebhookHTTPHandler_MethodNotAllowed(t *testing.T) {
	// 目的: Stripe webhook endpoint が POST 以外を拒否し、service を呼ばないことを確認する。
	svc := &billingHandlerTestUsecase{}
	h := NewBillingWebhookHTTPHandler(svc, nil)
	req := httptest.NewRequest(http.MethodGet, "/stripe/webhook", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, http.MethodPost, rec.Header().Get("Allow"))
	assert.Zero(t, svc.webhookCalls)
}

func TestBillingWebhookHTTPHandler_InvalidSignature_ReturnsBadRequest(t *testing.T) {
	// 目的: provider が署名エラーを返したとき、HTTP として 400 を返すことを確認する。
	// Stripe 側の再送/監視で扱いやすいように、内部エラーと区別する。
	svc := &billingHandlerTestUsecase{webhookErr: domain.ErrBillingWebhookSignatureInvalid}
	h := NewBillingWebhookHTTPHandler(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", bytes.NewBufferString(`{"id":"evt_1"}`))
	req.Header.Set("Stripe-Signature", "sig-test")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, svc.webhookCalls)
	assert.Equal(t, `{"id":"evt_1"}`, string(svc.gotWebhookPayload))
	assert.Equal(t, "sig-test", svc.gotWebhookSignature)
}

func TestBillingWebhookHTTPHandler_ServiceError_ReturnsInternalServerError(t *testing.T) {
	// 目的: 署名エラー以外の webhook 処理失敗は 500 として返すことを確認する。
	// 課金状態の適用失敗を Stripe 側の retry 対象にするための境界テスト。
	svc := &billingHandlerTestUsecase{webhookErr: errors.New("apply failed")}
	h := NewBillingWebhookHTTPHandler(svc, nil)
	req := httptest.NewRequest(http.MethodPost, "/stripe/webhook", bytes.NewBufferString(`{"id":"evt_1"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, 1, svc.webhookCalls)
}

type billingHandlerTestUsecase struct {
	createCheckoutSession  *domain.BillingCheckoutSession
	createCheckoutErr      error
	createCheckoutCalls    int
	gotCheckoutAccountID   string
	gotCheckoutActorUserID string
	gotCheckoutPlan        domain.BillingPlan
	gotCheckoutCurrency    domain.BillingCurrency

	createPortalSession  *domain.BillingPortalSession
	createPortalErr      error
	createPortalCalls    int
	gotPortalAccountID   string
	gotPortalActorUserID string

	getUsageCalls           int
	recordUsageCalls        int
	gotUsageEvent           *domain.UsageEvent
	updateBudgetCalls       int
	listInvoicesCalls       int
	listPaymentMethodsCalls int
	webhookCalls            int
	webhookErr              error
	gotWebhookPayload       []byte
	gotWebhookSignature     string
}

func (u *billingHandlerTestUsecase) GetBillingAccount(ctx context.Context, accountID, actorUserID string) (*domain.Account, error) {
	return nil, domain.ErrNotImplemented
}

func (u *billingHandlerTestUsecase) CreateCheckoutSession(ctx context.Context, accountID, actorUserID string, plan domain.BillingPlan, currency domain.BillingCurrency) (*domain.BillingCheckoutSession, error) {
	u.createCheckoutCalls++
	u.gotCheckoutAccountID = accountID
	u.gotCheckoutActorUserID = actorUserID
	u.gotCheckoutPlan = plan
	u.gotCheckoutCurrency = currency
	return u.createCheckoutSession, u.createCheckoutErr
}

func (u *billingHandlerTestUsecase) CreatePortalSession(ctx context.Context, accountID, actorUserID string) (*domain.BillingPortalSession, error) {
	u.createPortalCalls++
	u.gotPortalAccountID = accountID
	u.gotPortalActorUserID = actorUserID
	return u.createPortalSession, u.createPortalErr
}

func (u *billingHandlerTestUsecase) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	u.webhookCalls++
	u.gotWebhookPayload = append([]byte(nil), payload...)
	u.gotWebhookSignature = signature
	return u.webhookErr
}

func (u *billingHandlerTestUsecase) GetUsage(ctx context.Context, accountID, actorUserID string, periodStart, periodEnd string) (*domain.UsageReport, error) {
	u.getUsageCalls++
	return &domain.UsageReport{AccountID: accountID, TotalCost: "0.00", Currency: "usd"}, nil
}

func (u *billingHandlerTestUsecase) RecordUsage(ctx context.Context, ev *domain.UsageEvent) (*domain.UsageRecordResult, error) {
	u.recordUsageCalls++
	if ev != nil {
		copy := *ev
		u.gotUsageEvent = &copy
	}
	return &domain.UsageRecordResult{EventID: ev.EventID, Cost: "0.00"}, nil
}

func (u *billingHandlerTestUsecase) UpdateBudget(ctx context.Context, accountID, actorUserID string, budgetLimit string) (string, error) {
	u.updateBudgetCalls++
	return budgetLimit, nil
}

func (u *billingHandlerTestUsecase) ListInvoices(ctx context.Context, accountID, actorUserID string, limit int) (*domain.InvoiceList, error) {
	u.listInvoicesCalls++
	return &domain.InvoiceList{UpcomingAmount: "0.00"}, nil
}

func (u *billingHandlerTestUsecase) ListPaymentMethods(ctx context.Context, accountID, actorUserID string) ([]*domain.PaymentMethod, error) {
	u.listPaymentMethodsCalls++
	return nil, nil
}
