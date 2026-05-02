package service

import (
	"context"

	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type ItemService struct {
	repo repository.ItemRepository
	tree repository.TreeRepository
}

func NewItemService(repo repository.ItemRepository, tree repository.TreeRepository) *ItemService {
	return &ItemService{repo: repo, tree: tree}
}

func (s *ItemService) CreateItem(ctx context.Context, workspaceID, label, description, parentID, createdBy string) (*domain.Item, error) {
	if _, err := s.tree.GetOrCreateTree(ctx, workspaceID); err != nil {
		return nil, err
	}
	item := s.repo.CreateItem(ctx, workspaceID, label, description, parentID, createdBy)
	if item == nil {
		return nil, domain.ErrNotFound
	}
	return item, nil
}
