package service

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type TreeService struct {
	repo repository.TreeRepository
}

func NewTreeService(repo repository.TreeRepository) *TreeService {
	return &TreeService{repo: repo}
}

func (s *TreeService) GetTreeByWorkspace(workspaceID string) ([]*domain.Item, error) {
	items, ok := s.repo.GetTreeByWorkspace(workspaceID)
	if !ok {
		return nil, ErrNotFound
	}
	return items, nil
}

func (s *TreeService) FindPaths(treeID, sourceItemID, targetItemID string, maxDepth, limit int) ([]*domain.Item, []domain.TreePath, error) {
	items, paths, ok := s.repo.FindPaths(treeID, sourceItemID, targetItemID, maxDepth, limit)
	if !ok {
		return nil, nil, ErrNotFound
	}
	return items, paths, nil
}

func (s *TreeService) GetOrCreateTree(workspaceID string) (*domain.Tree, error) {
	return s.repo.GetOrCreateTree(workspaceID)
}

func (s *TreeService) GetSubtree(rootItemID string, maxDepth int) ([]*domain.SubtreeItem, error) {
	return s.repo.GetSubtree(rootItemID, maxDepth)
}
