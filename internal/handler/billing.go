package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
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
