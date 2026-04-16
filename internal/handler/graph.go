package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/api/internal/service"
)

type GraphHandler struct {
	service    *service.GraphService
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
}

func NewGraphHandler(
	svc *service.GraphService,
	workspaceRepo repository.WorkspaceRepository,
	documentRepo repository.DocumentRepository,
) *GraphHandler {
	return &GraphHandler{
		service:    svc,
		workspaces: workspaceRepo,
		documents:  documentRepo,
	}
}

func (h *GraphHandler) GetGraph(ctx context.Context, req *connect.Request[graphv1.GetGraphRequest]) (*connect.Response[graphv1.GetGraphResponse], error) {
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	nodes, edges, err := h.service.GetGraphByWorkspace(req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	graph := &graphv1.Graph{
		WorkspaceId: req.Msg.GetWorkspaceId(),
	}
	nodeIDs := map[string]bool{}
	for _, node := range nodes {
		protoNode := toProtoNode(node)
		graph.Nodes = append(graph.Nodes, protoNode)
		nodeIDs[protoNode.GetId()] = true
	}
	for _, edge := range edges {
		if !nodeIDs[edge.SourceNodeID] || !nodeIDs[edge.TargetNodeID] {
			continue
		}
		graph.Edges = append(graph.Edges, toProtoEdge(edge))
	}
	return connect.NewResponse(&graphv1.GetGraphResponse{Graph: graph}), nil
}

func (h *GraphHandler) FindPaths(ctx context.Context, req *connect.Request[graphv1.FindPathsRequest]) (*connect.Response[graphv1.FindPathsResponse], error) {
	if req.Msg.GetSourceNodeId() == "" || req.Msg.GetTargetNodeId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("source_node_id and target_node_id are required"))
	}
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}

	graph, err := h.service.GetOrCreateGraph(req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	nodes, edges, paths, err := h.service.FindPaths(graph.GraphID, req.Msg.GetSourceNodeId(), req.Msg.GetTargetNodeId(), int(req.Msg.GetMaxDepth()), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	protoGraph := &graphv1.Graph{
		WorkspaceId:   req.Msg.GetWorkspaceId(),
		CrossDocument: req.Msg.GetCrossDocument(),
	}
	for _, node := range nodes {
		protoGraph.Nodes = append(protoGraph.Nodes, toProtoNode(node))
	}
	for _, edge := range edges {
		protoGraph.Edges = append(protoGraph.Edges, toProtoEdge(edge))
	}

	res := connect.NewResponse(&graphv1.FindPathsResponse{Graph: protoGraph})
	for _, path := range paths {
		res.Msg.Paths = append(res.Msg.Paths, &graphv1.GraphPath{
			NodeIds:  path.NodeIDs,
			HopCount: int32(path.HopCount),
			EvidenceRef: &graphv1.PathEvidenceRef{
				SourceDocumentIds: path.Evidence.SourceDocumentIDs,
			},
		})
	}
	return res, nil
}
