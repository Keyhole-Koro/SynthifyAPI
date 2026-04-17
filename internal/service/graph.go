package service

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type GraphService struct {
	repo repository.GraphRepository
}

func NewGraphService(repo repository.GraphRepository) *GraphService {
	return &GraphService{repo: repo}
}

func (s *GraphService) GetGraphByWorkspace(workspaceID string) ([]*domain.Node, []*domain.Edge, error) {
	nodes, edges, ok := s.repo.GetGraphByWorkspace(workspaceID)
	if !ok {
		return nil, nil, ErrNotFound
	}
	return nodes, edges, nil
}

func (s *GraphService) FindPaths(graphID, sourceNodeID, targetNodeID string, maxDepth, limit int) ([]*domain.Node, []*domain.Edge, []domain.GraphPath, error) {
	nodes, edges, paths, ok := s.repo.FindPaths(graphID, sourceNodeID, targetNodeID, maxDepth, limit)
	if !ok {
		return nil, nil, nil, ErrNotFound
	}
	return nodes, edges, paths, nil
}

func (s *GraphService) GetOrCreateGraph(workspaceID string) (*domain.Graph, error) {
	return s.repo.GetOrCreateGraph(workspaceID)
}

func (s *GraphService) GetSubtree(rootNodeID string, maxDepth int) ([]*domain.SubtreeNode, []*domain.Edge, error) {
	return s.repo.GetSubtree(rootNodeID, maxDepth)
}
