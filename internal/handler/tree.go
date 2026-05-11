package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/transport/connect"
	"github.com/synthify/backend/packages/shared/transport/connect/mappers"
)

type TreeHandler struct {
	repo       repository.TreeRepository
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
}

func NewTreeHandler(
	treeRepo repository.TreeRepository,
	workspaceRepo repository.WorkspaceRepository,
	documentRepo repository.DocumentRepository,
) *TreeHandler {
	return &TreeHandler{
		repo:       treeRepo,
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
	items, err := h.repo.GetTreeByWorkspace(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connectutil.ToError(err)
	}

	tree := &treev1.Tree{
		WorkspaceId: req.Msg.GetWorkspaceId(),
	}
	for _, item := range items {
		protoItem := mappers.ToProtoItem(item)
		tree.Items = append(tree.Items, protoItem)
	}
	return connect.NewResponse(&treev1.GetTreeResponse{Tree: tree}), nil
}

func (h *TreeHandler) GetSubtree(ctx context.Context, req *connect.Request[treev1.GetSubtreeRequest]) (*connect.Response[treev1.GetSubtreeResponse], error) {
	wsID := req.Msg.GetWorkspaceId()
	if wsID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, wsID); err != nil {
		return nil, err
	}
	itemID := req.Msg.GetItemId()
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("item_id is required"))
	}
	maxDepth := int(req.Msg.GetMaxDepth())
	if maxDepth <= 0 {
		maxDepth = 3
	}
	items, err := h.repo.GetSubtree(ctx, itemID, maxDepth)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(items) == 0 {
		return nil, connectutil.ToError(domain.ErrNotFound)
	}
	if items[0].WorkspaceID != wsID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("item does not belong to workspace"))
	}
	protoItems := make([]*treev1.SubtreeItem, len(items))
	for i, item := range items {
		protoItems[i] = mappers.ToProtoSubtreeItem(item)
	}
	return connect.NewResponse(&treev1.GetSubtreeResponse{Items: protoItems}), nil
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

	tree, err := h.repo.GetOrCreateTree(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, connectutil.ToError(err)
	}

	items, paths, err := h.repo.FindPaths(ctx, tree.TreeID, req.Msg.GetSourceItemId(), req.Msg.GetTargetItemId(), int(req.Msg.GetMaxDepth()), int(req.Msg.GetLimit()))
	if err != nil {
		return nil, connectutil.ToError(err)
	}

	protoTree := &treev1.Tree{
		WorkspaceId:   req.Msg.GetWorkspaceId(),
		CrossDocument: req.Msg.GetCrossDocument(),
	}
	for _, item := range items {
		protoTree.Items = append(protoTree.Items, mappers.ToProtoItem(item))
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
