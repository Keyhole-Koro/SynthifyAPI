package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/internal/middleware"
)

func currentUser(ctx context.Context) (middleware.AuthUser, error) {
	user, ok := middleware.CurrentUser(ctx)
	if !ok || user.ID == "" {
		return middleware.AuthUser{}, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return user, nil
}

func authorizeWorkspace(ctx context.Context, repo repository.WorkspaceRepository, workspaceID string) error {
	user, err := currentUser(ctx)
	if err != nil {
		return err
	}
	if !repo.IsWorkspaceAccessible(workspaceID, user.ID) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("workspace access denied"))
	}
	return nil
}

func authorizeDocument(
	ctx context.Context,
	workspaceRepo repository.WorkspaceRepository,
	documentRepo repository.DocumentRepository,
	documentID string,
	expectedWorkspaceID string,
) error {
	doc, ok := documentRepo.GetDocument(documentID)
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("document not found"))
	}
	if expectedWorkspaceID != "" && doc.WorkspaceID != expectedWorkspaceID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("document does not belong to workspace"))
	}
	return authorizeWorkspace(ctx, workspaceRepo, doc.WorkspaceID)
}

func authorizeNode(
	ctx context.Context,
	workspaceRepo repository.WorkspaceRepository,
	nodeRepo repository.NodeRepository,
	nodeID string,
	workspaceID string,
) error {
	_, _, ok := nodeRepo.GetNode(nodeID)
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("node not found"))
	}
	return authorizeWorkspace(ctx, workspaceRepo, workspaceID)
}
