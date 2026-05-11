package service

import (
	"context"
	"errors"

	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type WorkspaceService struct {
	accounts   repository.AccountRepository
	workspaces repository.WorkspaceRepository
	logger     applog.Logger
}

func NewWorkspaceService(accounts repository.AccountRepository, workspaces repository.WorkspaceRepository, logger applog.Logger) *WorkspaceService {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &WorkspaceService{accounts: accounts, workspaces: workspaces, logger: logger}
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, id, userID string) (*domain.Workspace, error) {
	if !s.workspaces.IsWorkspaceAccessible(ctx, id, userID) {
		return nil, domain.ErrNotFound
	}
	ws, err := s.workspaces.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
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
