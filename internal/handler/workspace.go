package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/handlerutil"
	"github.com/synthify/backend/packages/shared/mappers"
	"github.com/synthify/backend/packages/shared/repository"
)

type WorkspaceHandler struct {
	service    *service.WorkspaceService
	workspaces repository.WorkspaceRepository
}

func NewWorkspaceHandler(svc *service.WorkspaceService, workspaceRepo repository.WorkspaceRepository) *WorkspaceHandler {
	return &WorkspaceHandler{service: svc, workspaces: workspaceRepo}
}

func (h *WorkspaceHandler) ListWorkspaces(ctx context.Context, _ *connect.Request[treev1.ListWorkspacesRequest]) (*connect.Response[treev1.ListWorkspacesResponse], error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := h.workspaces.ListWorkspacesByUser(ctx, user.ID)
	res := connect.NewResponse(&treev1.ListWorkspacesResponse{})
	for _, workspace := range workspaces {
		res.Msg.Workspaces = append(res.Msg.Workspaces, mappers.ToProtoWorkspace(workspace))
	}
	return res, nil
}

func (h *WorkspaceHandler) GetWorkspace(ctx context.Context, req *connect.Request[treev1.GetWorkspaceRequest]) (*connect.Response[treev1.GetWorkspaceResponse], error) {
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	workspace, err := h.service.GetWorkspace(ctx, req.Msg.GetWorkspaceId(), user.ID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.GetWorkspaceResponse{
		Workspace: mappers.ToProtoWorkspace(workspace),
	}), nil
}

func (h *WorkspaceHandler) CreateWorkspace(ctx context.Context, req *connect.Request[treev1.CreateWorkspaceRequest]) (*connect.Response[treev1.CreateWorkspaceResponse], error) {
	if req.Msg.GetName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := h.service.CreateWorkspace(ctx, req.Msg.GetName(), user.ID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.CreateWorkspaceResponse{
		Workspace: mappers.ToProtoWorkspace(ws),
	}), nil
}

func (h *WorkspaceHandler) InviteMember(_ context.Context, _ *connect.Request[treev1.InviteMemberRequest]) (*connect.Response[treev1.InviteMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) UpdateMemberRole(_ context.Context, _ *connect.Request[treev1.UpdateMemberRoleRequest]) (*connect.Response[treev1.UpdateMemberRoleResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) RemoveMember(_ context.Context, _ *connect.Request[treev1.RemoveMemberRequest]) (*connect.Response[treev1.RemoveMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) TransferOwnership(_ context.Context, _ *connect.Request[treev1.TransferOwnershipRequest]) (*connect.Response[treev1.TransferOwnershipResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace ownership is managed at account level"))
}
