package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/transport/connect"
)

type BillingHandler struct {
	service service.BillingUsecase
}

func NewBillingHandler(svc service.BillingUsecase) *BillingHandler {
	return &BillingHandler{service: svc}
}

func NewBillingWebhookHTTPHandler(svc service.BillingUsecase, logger applog.Logger) http.HandlerFunc {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			logger.Warn(r.Context(), "billing.webhook.read_failed", err, map[string]any{"path": r.URL.Path})
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if err := svc.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, domain.ErrBillingWebhookSignatureInvalid) {
				status = http.StatusBadRequest
			}
			http.Error(w, "webhook failed", status)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func (h *BillingHandler) GetBillingAccount(ctx context.Context, req *connect.Request[treev1.GetBillingAccountRequest]) (*connect.Response[treev1.GetBillingAccountResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	account, err := h.service.GetBillingAccount(ctx, req.Msg.GetAccountId(), user.ID)
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.GetBillingAccountResponse{
		AccountId:              account.AccountID,
		Plan:                   account.Plan,
		BillingStatus:          account.BillingStatus,
		StorageQuotaBytes:      account.StorageQuotaBytes,
		StorageUsedBytes:       account.StorageUsedBytes,
		MaxFileSizeBytes:       account.MaxFileSizeBytes,
		StripePriceId:          account.StripePriceID,
		BillingCurrency:        account.BillingCurrency,
		BillingInterval:        account.BillingInterval,
		BillingAmountMinor:     account.BillingAmountMinor,
		CurrentPeriodEnd:       account.CurrentPeriodEnd,
		CancelAtPeriodEnd:      account.CancelAtPeriodEnd,
		BudgetLimit:            minorToDecimal(account.BudgetLimitMinor),
		CurrentPeriodUsage:     minorToDecimal(account.CurrentPeriodUsageMinor),
		CurrentPeriodStartedAt: account.CurrentPeriodStartedAt,
		BudgetExceeded:         account.BudgetExceeded,
	}), nil
}

// minorToDecimal converts cents (or equivalent minor unit) to a decimal string like "12.34".
func minorToDecimal(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, abs64(minor%100))
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func (h *BillingHandler) CreateCheckoutSession(ctx context.Context, req *connect.Request[treev1.CreateCheckoutSessionRequest]) (*connect.Response[treev1.CreateCheckoutSessionResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	session, err := h.service.CreateCheckoutSession(ctx, req.Msg.GetAccountId(), user.ID, domain.BillingPlanPro, domain.BillingCurrency(req.Msg.GetCurrency()))
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.CreateCheckoutSessionResponse{
		CheckoutUrl: session.URL,
	}), nil
}

func (h *BillingHandler) CreatePortalSession(ctx context.Context, req *connect.Request[treev1.CreatePortalSessionRequest]) (*connect.Response[treev1.CreatePortalSessionResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	session, err := h.service.CreatePortalSession(ctx, req.Msg.GetAccountId(), user.ID)
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.CreatePortalSessionResponse{
		PortalUrl: session.URL,
	}), nil
}

// =========================================================
// Usage-Based Billing handlers
// =========================================================

func (h *BillingHandler) GetUsage(ctx context.Context, req *connect.Request[treev1.GetUsageRequest]) (*connect.Response[treev1.GetUsageResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	report, err := h.service.GetUsage(ctx, req.Msg.GetAccountId(), user.ID, req.Msg.GetPeriodStart(), req.Msg.GetPeriodEnd())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	resp := &treev1.GetUsageResponse{
		AccountId:   report.AccountID,
		PeriodStart: report.PeriodStart,
		PeriodEnd:   report.PeriodEnd,
		TotalCost:   report.TotalCost,
		Currency:    report.Currency,
	}
	for _, m := range report.ByModel {
		resp.ByModel = append(resp.ByModel, &treev1.ModelUsage{
			Model:        m.Model,
			InputTokens:  m.InputTokens,
			OutputTokens: m.OutputTokens,
			InputCost:    m.InputCost,
			OutputCost:   m.OutputCost,
			TotalCost:    m.TotalCost,
			EventCount:   m.EventCount,
		})
	}
	for _, d := range report.ByDay {
		resp.ByDay = append(resp.ByDay, &treev1.DailyUsage{
			Date:      d.Date,
			TotalCost: d.TotalCost,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *BillingHandler) RecordUsage(ctx context.Context, req *connect.Request[treev1.RecordUsageRequest]) (*connect.Response[treev1.RecordUsageResponse], error) {
	// 内部 RPC: worker -> API のサービストークン認証のみ受け付ける。
	// SYNTHIFY_INTERNAL_SERVICE_TOKEN が未設定の環境では middleware が token を要求しない
	// (= service call を立てない) ので、すべての RecordUsage は拒否される。
	if !middleware.IsServiceCall(ctx) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("service token required"))
	}
	if req.Msg.GetAccountId() == "" || req.Msg.GetModel() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id and model are required"))
	}
	ev := &domain.UsageEvent{
		AccountID:    req.Msg.GetAccountId(),
		WorkspaceID:  req.Msg.GetWorkspaceId(),
		JobID:        req.Msg.GetJobId(),
		Model:        req.Msg.GetModel(),
		InputTokens:  req.Msg.GetInputTokens(),
		OutputTokens: req.Msg.GetOutputTokens(),
	}
	result, err := h.service.RecordUsage(ctx, ev)
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.RecordUsageResponse{
		EventId:        result.EventID,
		Cost:           result.Cost,
		BudgetExceeded: result.BudgetExceeded,
	}), nil
}

func (h *BillingHandler) UpdateBudget(ctx context.Context, req *connect.Request[treev1.UpdateBudgetRequest]) (*connect.Response[treev1.UpdateBudgetResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	limit, err := h.service.UpdateBudget(ctx, req.Msg.GetAccountId(), user.ID, req.Msg.GetBudgetLimit())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.UpdateBudgetResponse{BudgetLimit: limit}), nil
}

func (h *BillingHandler) ListInvoices(ctx context.Context, req *connect.Request[treev1.ListInvoicesRequest]) (*connect.Response[treev1.ListInvoicesResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	list, err := h.service.ListInvoices(ctx, req.Msg.GetAccountId(), user.ID, int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	resp := &treev1.ListInvoicesResponse{
		UpcomingAmount:    list.UpcomingAmount,
		UpcomingPeriodEnd: list.UpcomingPeriodEnd,
	}
	for _, inv := range list.Invoices {
		resp.Invoices = append(resp.Invoices, &treev1.Invoice{
			InvoiceId:        inv.InvoiceID,
			Amount:           inv.Amount,
			Currency:         inv.Currency,
			Status:           inv.Status,
			HostedInvoiceUrl: inv.HostedInvoiceURL,
			InvoicePdfUrl:    inv.InvoicePDFURL,
			PeriodStart:      inv.PeriodStart,
			PeriodEnd:        inv.PeriodEnd,
			PaidAt:           inv.PaidAt,
			CreatedAt:        inv.CreatedAt,
		})
	}
	return connect.NewResponse(resp), nil
}

func (h *BillingHandler) ListPaymentMethods(ctx context.Context, req *connect.Request[treev1.ListPaymentMethodsRequest]) (*connect.Response[treev1.ListPaymentMethodsResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	methods, err := h.service.ListPaymentMethods(ctx, req.Msg.GetAccountId(), user.ID)
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	resp := &treev1.ListPaymentMethodsResponse{}
	for _, pm := range methods {
		resp.PaymentMethods = append(resp.PaymentMethods, &treev1.PaymentMethod{
			PaymentMethodId: pm.PaymentMethodID,
			Brand:           pm.Brand,
			Last4:           pm.Last4,
			ExpMonth:        pm.ExpMonth,
			ExpYear:         pm.ExpYear,
			IsDefault:       pm.IsDefault,
		})
	}
	return connect.NewResponse(resp), nil
}
