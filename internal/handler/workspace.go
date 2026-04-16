package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	"github.com/synthify/backend/api/internal/service"
)

type WorkspaceHandler struct {
	service *service.WorkspaceService
}

func NewWorkspaceHandler(svc *service.WorkspaceService) *WorkspaceHandler {
	return &WorkspaceHandler{service: svc}
}

func (h *WorkspaceHandler) ListWorkspaces(ctx context.Context, _ *connect.Request[graphv1.ListWorkspacesRequest]) (*connect.Response[graphv1.ListWorkspacesResponse], error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := h.service.ListWorkspaces(user.ID)
	res := connect.NewResponse(&graphv1.ListWorkspacesResponse{})
	for _, workspace := range workspaces {
		res.Msg.Workspaces = append(res.Msg.Workspaces, toProtoWorkspace(workspace))
	}
	return res, nil
}

func (h *WorkspaceHandler) GetWorkspace(ctx context.Context, req *connect.Request[graphv1.GetWorkspaceRequest]) (*connect.Response[graphv1.GetWorkspaceResponse], error) {
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	workspace, err := h.service.GetWorkspace(req.Msg.GetWorkspaceId(), user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, err)
	}
	return connect.NewResponse(&graphv1.GetWorkspaceResponse{
		Workspace: toProtoWorkspace(workspace),
	}), nil
}

func (h *WorkspaceHandler) CreateWorkspace(ctx context.Context, req *connect.Request[graphv1.CreateWorkspaceRequest]) (*connect.Response[graphv1.CreateWorkspaceResponse], error) {
	if req.Msg.GetName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	ws, err := h.service.CreateWorkspace(req.Msg.GetName(), user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&graphv1.CreateWorkspaceResponse{
		Workspace: toProtoWorkspace(ws),
	}), nil
}

func (h *WorkspaceHandler) InviteMember(_ context.Context, _ *connect.Request[graphv1.InviteMemberRequest]) (*connect.Response[graphv1.InviteMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) UpdateMemberRole(_ context.Context, _ *connect.Request[graphv1.UpdateMemberRoleRequest]) (*connect.Response[graphv1.UpdateMemberRoleResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) RemoveMember(_ context.Context, _ *connect.Request[graphv1.RemoveMemberRequest]) (*connect.Response[graphv1.RemoveMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace membership is managed at account level"))
}

func (h *WorkspaceHandler) TransferOwnership(_ context.Context, _ *connect.Request[graphv1.TransferOwnershipRequest]) (*connect.Response[graphv1.TransferOwnershipResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("workspace ownership is managed at account level"))
}

func toProtoWorkspace(workspace *domain.Workspace) *graphv1.Workspace {
	return &graphv1.Workspace{
		WorkspaceId: workspace.WorkspaceID,
		Name:        workspace.Name,
		CreatedAt:   workspace.CreatedAt,
	}
}
