package service

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type NodeService struct {
	repo  repository.NodeRepository
	graph repository.GraphRepository
}

func NewNodeService(repo repository.NodeRepository, graph repository.GraphRepository) *NodeService {
	return &NodeService{repo: repo, graph: graph}
}

func (s *NodeService) GetGraphEntityDetail(nodeID string) (*domain.Node, []*domain.Edge, error) {
	node, edges, ok := s.repo.GetNode(nodeID)
	if !ok {
		return nil, nil, ErrNotFound
	}
	return node, edges, nil
}

func (s *NodeService) CreateNode(workspaceID, label, description, parentNodeID, createdBy string) (*domain.Node, error) {
	graph, err := s.graph.GetOrCreateGraph(workspaceID)
	if err != nil {
		return nil, err
	}
	node := s.repo.CreateNode(graph.GraphID, label, description, parentNodeID, createdBy)
	if node == nil {
		return nil, ErrNotFound
	}
	return node, nil
}

func (s *NodeService) ApproveAlias(workspaceID, canonicalNodeID, aliasNodeID string) error {
	if !s.repo.ApproveAlias(workspaceID, canonicalNodeID, aliasNodeID) {
		return ErrNotFound
	}
	return nil
}

func (s *NodeService) RejectAlias(workspaceID, canonicalNodeID, aliasNodeID string) error {
	if !s.repo.RejectAlias(workspaceID, canonicalNodeID, aliasNodeID) {
		return ErrNotFound
	}
	return nil
}
