package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	connect "connectrpc.com/connect"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/api/internal/service"
)

type TreeHandler struct {
	service    *service.TreeService
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
}

func NewTreeHandler(
	svc *service.TreeService,
	workspaceRepo repository.WorkspaceRepository,
	documentRepo repository.DocumentRepository,
) *TreeHandler {
	return &TreeHandler{
		service:    svc,
		workspaces: workspaceRepo,
		documents:  documentRepo,
	}
}

func (h *TreeHandler) GetTree(ctx context.Context, req *connect.Request[treev1.GetTreeRequest]) (*connect.Response[treev1.GetTreeResponse], error) {
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	items, err := h.service.GetTreeByWorkspace(req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	tree := &treev1.Tree{
		WorkspaceId: req.Msg.GetWorkspaceId(),
	}
	for _, item := range items {
		protoItem := toProtoItem(item)
		tree.Items = append(tree.Items, protoItem)
	}
	return connect.NewResponse(&treev1.GetTreeResponse{Tree: tree}), nil
}

func (h *TreeHandler) GetSubtreeHTTP(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspace_id")
	itemID := r.URL.Query().Get("item_id")
	if workspaceID == "" || itemID == "" {
		writeError(w, http.StatusBadRequest, "workspace_id and item_id are required")
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

	items, err := h.service.GetSubtree(itemID, maxDepth)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get subtree")
		return
	}

	type respItem struct {
		ID          string   `json:"id"`
		Label       string   `json:"label"`
		Level       int      `json:"level"`
		Description string   `json:"description"`
		SummaryHTML string   `json:"summary_html,omitempty"`
		HasChildren bool     `json:"has_children"`
		ParentID    string   `json:"parent_id,omitempty"`
		ChildIDs    []string `json:"child_ids,omitempty"`
	}

	out := make([]respItem, 0, len(items))
	for _, n := range items {
		out = append(out, respItem{
			ID:          n.ItemID,
			Label:       n.Label,
			Level:       n.Level,
			Description: n.Description,
			SummaryHTML: n.SummaryHTML,
			HasChildren: n.HasChildren,
			ParentID:    n.ParentID,
			ChildIDs:    n.ChildIDs,
		})
	}
	writeJSON(w, out)
}

func (h *TreeHandler) FindPaths(ctx context.Context, req *connect.Request[treev1.FindPathsRequest]) (*connect.Response[treev1.FindPathsResponse], error) {
	if req.Msg.GetSourceItemId() == "" || req.Msg.GetTargetItemId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("source_item_id and target_item_id are required"))
	}
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}

	tree, err := h.service.GetOrCreateTree(req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	items, paths, err := h.service.FindPaths(tree.TreeID, req.Msg.GetSourceItemId(), req.Msg.GetTargetItemId(), int(req.Msg.GetMaxDepth()), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	protoTree := &treev1.Tree{
		WorkspaceId:   req.Msg.GetWorkspaceId(),
		CrossDocument: req.Msg.GetCrossDocument(),
	}
	for _, item := range items {
		protoTree.Items = append(protoTree.Items, toProtoItem(item))
	}

	res := connect.NewResponse(&treev1.FindPathsResponse{Tree: protoTree})
	for _, path := range paths {
		res.Msg.Paths = append(res.Msg.Paths, &treev1.TreePath{
			ItemIds:  path.ItemIDs,
			HopCount: int32(path.HopCount),
			EvidenceRef: &treev1.PathEvidenceRef{
				SourceDocumentIds: path.Evidence.SourceDocumentIDs,
			},
		})
	}
	return res, nil
}
