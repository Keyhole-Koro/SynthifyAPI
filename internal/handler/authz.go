package handler

import (
	"context"
	"errors"
	"log"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/transport/connect"
)

func currentUser(ctx context.Context) (middleware.AuthUser, error) {
	user, ok := middleware.CurrentUser(ctx)
	if !ok {
		log.Printf("currentUser: user not found in context")
		return middleware.AuthUser{}, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if user.ID == "" {
		log.Printf("currentUser: user ID is empty in context")
		return middleware.AuthUser{}, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return user, nil
}

func authorizeWorkspace(ctx context.Context, repo repository.WorkspaceRepository, workspaceID string) error {
	if middleware.AnonymousReadAllowed(ctx) {
		return nil
	}
	user, err := currentUser(ctx)
	if err != nil {
		return err
	}
	if !repo.IsWorkspaceAccessible(ctx, workspaceID, user.ID) {
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
	doc, err := documentRepo.GetDocument(ctx, documentID)
	if err != nil {
		return connectutil.ToError(err)
	}
	if expectedWorkspaceID != "" && doc.WorkspaceID != expectedWorkspaceID {
		return connect.NewError(connect.CodePermissionDenied, errors.New("document does not belong to workspace"))
	}
	return authorizeWorkspace(ctx, workspaceRepo, doc.WorkspaceID)
}

func authorizeItem(
	ctx context.Context,
	workspaceRepo repository.WorkspaceRepository,
	itemRepo repository.ItemRepository,
	itemID string,
	workspaceID string,
) error {
	_, err := itemRepo.GetItem(ctx, itemID)
	if err != nil {
		return connectutil.ToError(err)
	}
	return authorizeWorkspace(ctx, workspaceRepo, workspaceID)
}
