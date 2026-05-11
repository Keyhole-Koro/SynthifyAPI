package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
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

func (h *BillingHandler) CreateCheckoutSession(ctx context.Context, req *connect.Request[treev1.CreateCheckoutSessionRequest]) (*connect.Response[treev1.CreateCheckoutSessionResponse], error) {
	if req.Msg.GetAccountId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("account_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	session, err := h.service.CreateCheckoutSession(ctx, req.Msg.GetAccountId(), user.ID, domain.BillingPlanPro)
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
