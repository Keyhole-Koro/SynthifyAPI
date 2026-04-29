package service

import (
	"context"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type ItemService struct {
	repo repository.ItemRepository
	tree repository.TreeRepository
}

func NewItemService(repo repository.ItemRepository, tree repository.TreeRepository) *ItemService {
	return &ItemService{repo: repo, tree: tree}
}

func (s *ItemService) GetTreeEntityDetail(ctx context.Context, itemID string) (*domain.Item, error) {
	item, ok := s.repo.GetItem(ctx, itemID)
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (s *ItemService) CreateItem(ctx context.Context, workspaceID, label, description, parentID, createdBy string) (*domain.Item, error) {
	if _, err := s.tree.GetOrCreateTree(ctx, workspaceID); err != nil {
		return nil, err
	}
	item := s.repo.CreateItem(ctx, workspaceID, label, description, parentID, createdBy)
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

func (s *ItemService) ApproveAlias(ctx context.Context, workspaceID, canonicalItemID, aliasItemID string) error {
	if !s.repo.ApproveAlias(ctx, workspaceID, canonicalItemID, aliasItemID) {
		return ErrNotFound
	}
	return nil
}

func (s *ItemService) RejectAlias(ctx context.Context, workspaceID, canonicalItemID, aliasItemID string) error {
	if !s.repo.RejectAlias(ctx, workspaceID, canonicalItemID, aliasItemID) {
		return ErrNotFound
	}
	return nil
}
