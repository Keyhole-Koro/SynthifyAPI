package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/api/internal/service"
)

type NodeHandler struct {
	service    *service.NodeService
	workspaces repository.WorkspaceRepository
	nodes      repository.NodeRepository
}

func NewNodeHandler(
	svc *service.NodeService,
	workspaceRepo repository.WorkspaceRepository,
	nodeRepo repository.NodeRepository,
) *NodeHandler {
	return &NodeHandler{
		service:    svc,
		workspaces: workspaceRepo,
		nodes:      nodeRepo,
	}
}

func (h *NodeHandler) GetGraphEntityDetail(ctx context.Context, req *connect.Request[graphv1.GetGraphEntityDetailRequest]) (*connect.Response[graphv1.GetGraphEntityDetailResponse], error) {
	if req.Msg.GetTargetRef() == nil || req.Msg.GetTargetRef().GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_ref.id is required"))
	}
	if err := authorizeNode(ctx, h.workspaces, h.nodes, req.Msg.GetTargetRef().GetId(), req.Msg.GetTargetRef().GetWorkspaceId()); err != nil {
		return nil, err
	}

	node, relatedEdges, err := h.service.GetGraphEntityDetail(req.Msg.GetTargetRef().GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	detail := &graphv1.GraphEntityDetail{
		Ref: &graphv1.EntityRef{
			WorkspaceId: req.Msg.GetTargetRef().GetWorkspaceId(),
			Scope:       graphv1.GraphProjectionScope_GRAPH_PROJECTION_SCOPE_DOCUMENT,
			Id:          node.NodeID,
		},
		Node: toProtoNode(node),
		Evidence: &graphv1.GraphEntityEvidence{
			SourceDocumentIds: []string{},
		},
	}
	for _, edge := range relatedEdges {
		detail.RelatedEdges = append(detail.RelatedEdges, toProtoEdge(edge))
	}
	return connect.NewResponse(&graphv1.GetGraphEntityDetailResponse{Detail: detail}), nil
}

func (h *NodeHandler) RecordNodeView(_ context.Context, _ *connect.Request[graphv1.RecordNodeViewRequest]) (*connect.Response[graphv1.RecordNodeViewResponse], error) {
	// Presence is managed in Firestore, so the backend does not write to Postgres here.
	return connect.NewResponse(&graphv1.RecordNodeViewResponse{}), nil
}

func (h *NodeHandler) CreateNode(ctx context.Context, req *connect.Request[graphv1.CreateNodeRequest]) (*connect.Response[graphv1.CreateNodeResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetLabel() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id and label are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	node, err := h.service.CreateNode(req.Msg.GetWorkspaceId(), req.Msg.GetLabel(), req.Msg.GetDescription(), req.Msg.GetParentNodeId(), user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&graphv1.CreateNodeResponse{Node: toProtoNode(node)}), nil
}

func (h *NodeHandler) GetUserNodeActivity(_ context.Context, _ *connect.Request[graphv1.GetUserNodeActivityRequest]) (*connect.Response[graphv1.GetUserNodeActivityResponse], error) {
	// Node activity has already been moved to Firestore presence.
	return connect.NewResponse(&graphv1.GetUserNodeActivityResponse{
		Activity: &graphv1.UserNodeActivity{},
	}), nil
}

func (h *NodeHandler) ApproveAlias(ctx context.Context, req *connect.Request[graphv1.ApproveAliasRequest]) (*connect.Response[graphv1.ApproveAliasResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetCanonicalNodeId() == "" || req.Msg.GetAliasNodeId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id, canonical_node_id, and alias_node_id are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	if err := h.service.ApproveAlias(req.Msg.GetWorkspaceId(), req.Msg.GetCanonicalNodeId(), req.Msg.GetAliasNodeId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&graphv1.ApproveAliasResponse{
		CanonicalNodeId: req.Msg.GetCanonicalNodeId(),
		AliasNodeId:     req.Msg.GetAliasNodeId(),
		MergeStatus:     "approved",
	}), nil
}

func (h *NodeHandler) RejectAlias(ctx context.Context, req *connect.Request[graphv1.RejectAliasRequest]) (*connect.Response[graphv1.RejectAliasResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetCanonicalNodeId() == "" || req.Msg.GetAliasNodeId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id, canonical_node_id, and alias_node_id are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	if err := h.service.RejectAlias(req.Msg.GetWorkspaceId(), req.Msg.GetCanonicalNodeId(), req.Msg.GetAliasNodeId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&graphv1.RejectAliasResponse{
		CanonicalNodeId: req.Msg.GetCanonicalNodeId(),
		AliasNodeId:     req.Msg.GetAliasNodeId(),
		MergeStatus:     "rejected",
	}), nil
}

func toProtoNode(node *domain.Node) *graphv1.Node {
	return &graphv1.Node{
		Id:          node.NodeID,
		Label:       node.Label,
		Level:       int32(node.Level),
		Description: node.Description,
		SummaryHtml: node.SummaryHTML,
		CreatedAt:   node.CreatedAt,
		Scope:       graphv1.GraphProjectionScope_GRAPH_PROJECTION_SCOPE_DOCUMENT,
	}
}

func toProtoEdge(edge *domain.Edge) *graphv1.Edge {
	return &graphv1.Edge{
		Id:          edge.EdgeID,
		Source:      edge.SourceNodeID,
		Target:      edge.TargetNodeID,
		Type:        edge.EdgeType,
		Scope:       graphv1.GraphProjectionScope_GRAPH_PROJECTION_SCOPE_DOCUMENT,
		Description: edge.Description,
		CreatedAt:   edge.CreatedAt,
	}
}
