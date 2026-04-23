package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	treev1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1/treev1connect"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

func main() {
	port := firstNonEmpty(os.Getenv("PORT"), "18080")
	allowedOrigins := firstNonEmpty(os.Getenv("CORS_ALLOWED_ORIGINS"), "http://127.0.0.1:4173")

	store := mock.NewStore()

	mux := http.NewServeMux()
	mux.Handle(treev1connect.NewWorkspaceServiceHandler(&workspaceHandler{store: store}))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	h := middleware.Logger(middleware.CORS(allowedOrigins, middleware.WithAuth("e2e-project", mux)))
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Synthify E2E API listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type workspaceStore interface {
	GetOrCreateAccount(userID string) (*domain.Account, error)
	ListWorkspacesByUser(userID string) []*domain.Workspace
	CreateWorkspace(accountID, name string) *domain.Workspace
}

type workspaceHandler struct {
	store workspaceStore
}

func (h *workspaceHandler) ListWorkspaces(ctx context.Context, _ *connect.Request[treev1.ListWorkspacesRequest]) (*connect.Response[treev1.ListWorkspacesResponse], error) {
	user, ok := middleware.CurrentUser(ctx)
	if !ok || user.ID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth user"))
	}

	res := connect.NewResponse(&treev1.ListWorkspacesResponse{})
	for _, workspace := range h.store.ListWorkspacesByUser(user.ID) {
		res.Msg.Workspaces = append(res.Msg.Workspaces, toProtoWorkspace(workspace))
	}
	return res, nil
}

func (h *workspaceHandler) GetWorkspace(_ context.Context, _ *connect.Request[treev1.GetWorkspaceRequest]) (*connect.Response[treev1.GetWorkspaceResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not needed in e2e"))
}

func (h *workspaceHandler) CreateWorkspace(ctx context.Context, req *connect.Request[treev1.CreateWorkspaceRequest]) (*connect.Response[treev1.CreateWorkspaceResponse], error) {
	if req.Msg.GetName() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	user, ok := middleware.CurrentUser(ctx)
	if !ok || user.ID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing auth user"))
	}

	account, err := h.store.GetOrCreateAccount(user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	workspace := h.store.CreateWorkspace(account.AccountID, req.Msg.GetName())
	if workspace == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create workspace"))
	}

	return connect.NewResponse(&treev1.CreateWorkspaceResponse{
		Workspace: toProtoWorkspace(workspace),
	}), nil
}

func (h *workspaceHandler) InviteMember(context.Context, *connect.Request[treev1.InviteMemberRequest]) (*connect.Response[treev1.InviteMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not needed in e2e"))
}

func (h *workspaceHandler) UpdateMemberRole(context.Context, *connect.Request[treev1.UpdateMemberRoleRequest]) (*connect.Response[treev1.UpdateMemberRoleResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not needed in e2e"))
}

func (h *workspaceHandler) RemoveMember(context.Context, *connect.Request[treev1.RemoveMemberRequest]) (*connect.Response[treev1.RemoveMemberResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not needed in e2e"))
}

func (h *workspaceHandler) TransferOwnership(context.Context, *connect.Request[treev1.TransferOwnershipRequest]) (*connect.Response[treev1.TransferOwnershipResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not needed in e2e"))
}

func toProtoWorkspace(workspace *domain.Workspace) *treev1.Workspace {
	return &treev1.Workspace{
		WorkspaceId: workspace.WorkspaceID,
		Name:        workspace.Name,
		CreatedAt:   workspace.CreatedAt,
	}
}
