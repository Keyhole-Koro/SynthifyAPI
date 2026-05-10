package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/handlerutil"
)

type BillingHandler struct {
	service *service.BillingService
}

func NewBillingHandler(svc *service.BillingService) *BillingHandler {
	return &BillingHandler{service: svc}
}

func (h *BillingHandler) CreateCheckoutSession(ctx context.Context, req *connect.Request[treev1.CreateCheckoutSessionRequest]) (*connect.Response[treev1.CreateCheckoutSessionResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	checkoutURL, err := h.service.CreateCheckoutSession(ctx, req.Msg.GetAccountId(), user.ID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.CreateCheckoutSessionResponse{
		CheckoutUrl: checkoutURL,
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
	portalURL, err := h.service.CreatePortalSession(ctx, req.Msg.GetAccountId(), user.ID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.CreatePortalSessionResponse{
		PortalUrl: portalURL,
	}), nil
}
