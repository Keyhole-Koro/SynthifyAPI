package service

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type ItemService struct {
	repo  repository.ItemRepository
	tree repository.TreeRepository
}

func NewItemService(repo repository.ItemRepository, tree repository.TreeRepository) *ItemService {
	return &ItemService{repo: repo, tree: tree}
}

func (s *ItemService) GetTreeEntityDetail(itemID string) (*domain.Item, error) {
	item, ok := s.repo.GetItem(itemID)
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (s *ItemService) CreateItem(workspaceID, label, description, parentID, createdBy string) (*domain.Item, error) {
	if _, err := s.tree.GetOrCreateTree(workspaceID); err != nil {
		return nil, err
	}
	item := s.repo.CreateItem(workspaceID, label, description, parentID, createdBy)
	if item == nil {
		return nil, ErrNotFound
	}
	return item, nil
}

func (s *ItemService) ApproveAlias(workspaceID, canonicalItemID, aliasItemID string) error {
	if !s.repo.ApproveAlias(workspaceID, canonicalItemID, aliasItemID) {
		return ErrNotFound
	}
	return nil
}

func (s *ItemService) RejectAlias(workspaceID, canonicalItemID, aliasItemID string) error {
	if !s.repo.RejectAlias(workspaceID, canonicalItemID, aliasItemID) {
		return ErrNotFound
	}
	return nil
}
