package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	connect "connectrpc.com/connect"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
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

func (h *GraphHandler) GetSubtreeHTTP(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	nodeID := r.URL.Query().Get("node_id")
	if workspaceID == "" || nodeID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and node_id are required")
		return
	}
	maxDepth := 3
	if v := r.URL.Query().Get("max_depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxDepth = n
		}
	}

	user, ok := middleware.CurrentUser(r.Context())
	if !ok || user.ID == "" {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !h.workspaces.IsWorkspaceAccessible(workspaceID, user.ID) {
		writeError(w, http.StatusForbidden, "workspace access denied")
		return
	}

	nodes, edges, err := h.service.GetSubtree(nodeID, maxDepth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get subtree")
		return
	}

	type respNode struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Level       int    `json:"level"`
		EntityType  string `json:"entity_type,omitempty"`
		Description string `json:"description"`
		SummaryHTML string `json:"summary_html,omitempty"`
		HasChildren bool   `json:"has_children"`
	}
	type respEdge struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"type"`
	}
	type resp struct {
		Nodes []respNode `json:"nodes"`
		Edges []respEdge `json:"edges"`
	}

	out := resp{Nodes: make([]respNode, 0, len(nodes)), Edges: make([]respEdge, 0, len(edges))}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, respNode{
			ID:          n.NodeID,
			Label:       n.Label,
			Level:       n.Level,
			EntityType:  n.EntityType,
			Description: n.Description,
			SummaryHTML: n.SummaryHTML,
			HasChildren: n.HasChildren,
		})
	}
	for _, e := range edges {
		out.Edges = append(out.Edges, respEdge{
			ID:     e.EdgeID,
			Source: e.SourceNodeID,
			Target: e.TargetNodeID,
			Type:   e.EdgeType,
		})
	}
	writeJSON(w, out)
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
