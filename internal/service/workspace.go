package service

import (
	"context"
	"errors"

	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type WorkspaceService struct {
	accounts   repository.AccountRepository
	workspaces repository.WorkspaceRepository
}

func NewWorkspaceService(accounts repository.AccountRepository, workspaces repository.WorkspaceRepository) *WorkspaceService {
	return &WorkspaceService{accounts: accounts, workspaces: workspaces}
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, id, userID string) (*domain.Workspace, error) {
	if !s.workspaces.IsWorkspaceAccessible(ctx, id, userID) {
		return nil, domain.ErrNotFound
	}
	ws, ok := s.workspaces.GetWorkspace(ctx, id)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return ws, nil
}

func (s *WorkspaceService) CreateWorkspace(ctx context.Context, name, userID string) (*domain.Workspace, error) {
	account, err := s.accounts.GetOrCreateAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	ws := s.workspaces.CreateWorkspace(ctx, account.AccountID, name)
	if ws == nil {
		return nil, errors.New("failed to create workspace")
	}
	return ws, nil
}
