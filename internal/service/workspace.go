package service

import (
	"errors"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

var ErrNotFound = errors.New("not found")

type WorkspaceService struct {
	accounts   repository.AccountRepository
	workspaces repository.WorkspaceRepository
}

func NewWorkspaceService(accounts repository.AccountRepository, workspaces repository.WorkspaceRepository) *WorkspaceService {
	return &WorkspaceService{accounts: accounts, workspaces: workspaces}
}

func (s *WorkspaceService) ListWorkspaces(userID string) []*domain.Workspace {
	return s.workspaces.ListWorkspacesByUser(userID)
}

func (s *WorkspaceService) GetWorkspace(id, userID string) (*domain.Workspace, error) {
	if !s.workspaces.IsWorkspaceAccessible(id, userID) {
		return nil, ErrNotFound
	}
	ws, ok := s.workspaces.GetWorkspace(id)
	if !ok {
		return nil, ErrNotFound
	}
	return ws, nil
}

func (s *WorkspaceService) CreateWorkspace(name, userID string) (*domain.Workspace, error) {
	account, err := s.accounts.GetOrCreateAccount(userID)
	if err != nil {
		return nil, err
	}
	ws := s.workspaces.CreateWorkspace(account.AccountID, name)
	if ws == nil {
		return nil, errors.New("failed to create workspace")
	}
	return ws, nil
}
