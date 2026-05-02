package handler

import (
	"context"
	"errors"
	"log"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository"
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
	doc, ok := documentRepo.GetDocument(ctx, documentID)
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("document not found"))
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
	_, ok := itemRepo.GetItem(ctx, itemID)
	if !ok {
		return connect.NewError(connect.CodeNotFound, errors.New("item not found"))
	}
	return authorizeWorkspace(ctx, workspaceRepo, workspaceID)
}
