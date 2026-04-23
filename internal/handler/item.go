package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/api/internal/service"
)

type ItemHandler struct {
	service    *service.ItemService
	workspaces repository.WorkspaceRepository
	items      repository.ItemRepository
}

func NewItemHandler(
	svc *service.ItemService,
	workspaceRepo repository.WorkspaceRepository,
	itemRepo repository.ItemRepository,
) *ItemHandler {
	return &ItemHandler{
		service:    svc,
		workspaces: workspaceRepo,
		items:      itemRepo,
	}
}

func (h *ItemHandler) GetTreeEntityDetail(ctx context.Context, req *connect.Request[treev1.GetTreeEntityDetailRequest]) (*connect.Response[treev1.GetTreeEntityDetailResponse], error) {
	if req.Msg.GetTargetRef() == nil || req.Msg.GetTargetRef().GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ref.id is required"))
	}
	if err := authorizeItem(ctx, h.workspaces, h.items, req.Msg.GetTargetRef().GetId(), req.Msg.GetTargetRef().GetWorkspaceId()); err != nil {
		return nil, err
	}

	item, err := h.service.GetTreeEntityDetail(req.Msg.GetTargetRef().GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	detail := &treev1.TreeEntityDetail{
		Ref: &treev1.EntityRef{
			WorkspaceId: req.Msg.GetTargetRef().GetWorkspaceId(),
			Scope:       item.Scope,
			Id:          item.ItemID,
		},
		Item: toProtoItem(item),
		Evidence: &treev1.TreeEntityEvidence{
			SourceDocumentIds: []string{},
		},
	}
	return connect.NewResponse(&treev1.GetTreeEntityDetailResponse{Detail: detail}), nil
}

func (h *ItemHandler) RecordItemView(_ context.Context, _ *connect.Request[treev1.RecordItemViewRequest]) (*connect.Response[treev1.RecordItemViewResponse], error) {
	// Presence is managed in Firestore, so the backend does not write to Postgres here.
	return connect.NewResponse(&treev1.RecordItemViewResponse{}), nil
}

func (h *ItemHandler) CreateItem(ctx context.Context, req *connect.Request[treev1.CreateItemRequest]) (*connect.Response[treev1.CreateItemResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetLabel() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id and label are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	item, err := h.service.CreateItem(req.Msg.GetWorkspaceId(), req.Msg.GetLabel(), req.Msg.GetDescription(), req.Msg.GetParentId(), user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&treev1.CreateItemResponse{Item: toProtoItem(item)}), nil
}

func (h *ItemHandler) GetUserItemActivity(_ context.Context, _ *connect.Request[treev1.GetUserItemActivityRequest]) (*connect.Response[treev1.GetUserItemActivityResponse], error) {
	// Item activity has already been moved to Firestore presence.
	return connect.NewResponse(&treev1.GetUserItemActivityResponse{
		Activity: &treev1.UserItemActivity{},
	}), nil
}

func (h *ItemHandler) ApproveAlias(ctx context.Context, req *connect.Request[treev1.ApproveAliasRequest]) (*connect.Response[treev1.ApproveAliasResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetCanonicalItemId() == "" || req.Msg.GetAliasItemId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id, canonical_item_id, and alias_item_id are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	if err := h.service.ApproveAlias(req.Msg.GetWorkspaceId(), req.Msg.GetCanonicalItemId(), req.Msg.GetAliasItemId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.ApproveAliasResponse{
		CanonicalItemId: req.Msg.GetCanonicalItemId(),
		AliasItemId:     req.Msg.GetAliasItemId(),
		MergeStatus:     "approved",
	}), nil
}

func (h *ItemHandler) RejectAlias(ctx context.Context, req *connect.Request[treev1.RejectAliasRequest]) (*connect.Response[treev1.RejectAliasResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetCanonicalItemId() == "" || req.Msg.GetAliasItemId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id, canonical_item_id, and alias_item_id are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	if err := h.service.RejectAlias(req.Msg.GetWorkspaceId(), req.Msg.GetCanonicalItemId(), req.Msg.GetAliasItemId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.RejectAliasResponse{
		CanonicalItemId: req.Msg.GetCanonicalItemId(),
		AliasItemId:     req.Msg.GetAliasItemId(),
		MergeStatus:     "rejected",
	}), nil
}

func toProtoItem(item *domain.Item) *treev1.Item {
	return &treev1.Item{
		Id:              item.ItemID,
		Label:           item.Label,
		Level:           int32(item.Level),
		Description:     item.Description,
		SummaryHtml:     item.SummaryHTML,
		CreatedAt:       item.CreatedAt,
		Scope:           item.Scope,
		ParentId:        item.ParentID,
		ChildIds:        item.ChildIDs,
		GovernanceState: item.GovernanceState,
	}
}
