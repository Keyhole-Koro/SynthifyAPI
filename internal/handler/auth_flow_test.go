package handler

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/apps/api/internal/service"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func TestWorkspaceHandler_AuthFlow(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner")
	handler := newTestWorkspaceHandler(store)

	t.Run("list requires authentication", func(t *testing.T) {
		resp, err := handler.ListWorkspaces(ctx, connect.NewRequest(&treev1.ListWorkspacesRequest{}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("get rejects another user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "stranger@example.com"})

		resp, err := handler.GetWorkspace(authedCtx, connect.NewRequest(&treev1.GetWorkspaceRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeNotFound)
	})

	t.Run("get allows owner", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

		resp, err := handler.GetWorkspace(authedCtx, connect.NewRequest(&treev1.GetWorkspaceRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, fixture.Workspace.WorkspaceID, resp.Msg.GetWorkspace().GetWorkspaceId())
	})
}

func TestDocumentHandler_AuthFlow(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateWorkspaceWithDocumentFixture(t, ctx, store, "owner")
	handler := newTestDocumentHandler(store)

	t.Run("list requires authentication", func(t *testing.T) {
		resp, err := handler.ListDocuments(ctx, connect.NewRequest(&treev1.ListDocumentsRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("get rejects another user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "stranger@example.com"})

		resp, err := handler.GetDocument(authedCtx, connect.NewRequest(&treev1.GetDocumentRequest{
			DocumentId: fixture.Document.DocumentID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("create uses authenticated user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

		resp, err := handler.CreateDocument(authedCtx, connect.NewRequest(&treev1.CreateDocumentRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Filename:    "owned.pdf",
			MimeType:    "application/pdf",
			FileSize:    42,
		}))

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "owner", resp.Msg.GetDocument().GetUploadedBy())
	})
}

func TestTreeHandler_AuthFlow(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateWorkspaceWithTreeFixture(t, ctx, store, "owner")
	handler := NewTreeHandler(store, store, store)

	t.Run("get tree requires authentication", func(t *testing.T) {
		resp, err := handler.GetTree(ctx, connect.NewRequest(&treev1.GetTreeRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("get tree rejects another user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "stranger@example.com"})

		resp, err := handler.GetTree(authedCtx, connect.NewRequest(&treev1.GetTreeRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("get tree allows owner", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

		resp, err := handler.GetTree(authedCtx, connect.NewRequest(&treev1.GetTreeRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
		}))

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, fixture.Workspace.WorkspaceID, resp.Msg.GetTree().GetWorkspaceId())
	})
}

func TestItemHandler_AuthFlow(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateWorkspaceWithTreeFixture(t, ctx, store, "owner")
	handler := newTestItemHandler(store)

	t.Run("create requires authentication", func(t *testing.T) {
		resp, err := handler.CreateItem(ctx, connect.NewRequest(&treev1.CreateItemRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Label:       "unauthenticated",
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("create rejects another user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "stranger@example.com"})

		resp, err := handler.CreateItem(authedCtx, connect.NewRequest(&treev1.CreateItemRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Label:       "stranger-item",
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("create records owner as creator", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

		resp, err := handler.CreateItem(authedCtx, connect.NewRequest(&treev1.CreateItemRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Label:       "owner-item",
		}))

		require.NoError(t, err)
		require.NotNil(t, resp)
		item, err := store.GetItem(ctx, resp.Msg.GetItem().GetId())
		require.NoError(t, err)
		assert.Equal(t, "owner", item.CreatedBy)
	})
}

func TestJobHandler_AuthFlow(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateWorkspaceWithProcessingJobFixture(t, ctx, store, "owner", treev1.JobType_JOB_TYPE_PROCESS_DOCUMENT)
	handler := NewJobHandler(store, store, store, nil)

	t.Run("status requires authentication", func(t *testing.T) {
		resp, err := handler.GetJobStatus(ctx, connect.NewRequest(&treev1.GetJobStatusRequest{
			JobId: fixture.Job.JobID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodeUnauthenticated)
	})

	t.Run("status rejects another user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "stranger@example.com"})

		resp, err := handler.GetJobStatus(authedCtx, connect.NewRequest(&treev1.GetJobStatusRequest{
			JobId: fixture.Job.JobID,
		}))

		assert.Nil(t, resp)
		assertConnectCode(t, err, connect.CodePermissionDenied)
	})

	t.Run("approval request uses authenticated user", func(t *testing.T) {
		authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

		resp, err := handler.RequestJobApproval(authedCtx, connect.NewRequest(&treev1.RequestJobApprovalRequest{
			JobId:  fixture.Job.JobID,
			Reason: "needs review",
		}))

		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, "owner", resp.Msg.GetRequest().GetRequestedBy())
	})
}

func newTestWorkspaceHandler(store *mock.Store) *WorkspaceHandler {
	svc := service.NewWorkspaceService(store, store, nil)
	return NewWorkspaceHandler(svc, store)
}

func newTestDocumentHandler(store *mock.Store) *DocumentHandler {
	sourceURL := func(workspaceID, documentID string) string {
		return "https://storage.example/" + workspaceID + "/" + documentID
	}
	uploadURL := func(workspaceID, objectName string) string {
		return "https://upload.example/" + workspaceID + "/" + objectName
	}
	svc := service.NewDocumentService(store, store, sourceURL, nil, nil, nil, nil)
	return NewDocumentHandler(svc, store, store, uploadURL)
}

func newTestItemHandler(store *mock.Store) *ItemHandler {
	svc := service.NewItemService(store, store, nil)
	return NewItemHandler(svc, store, store)
}
