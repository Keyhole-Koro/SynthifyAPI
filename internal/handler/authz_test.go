package handler

import (
	"context"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

// assertConnectCode fails the test if err is nil or does not carry the expected connect code.
func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	require.Error(t, err, "expected error with code %v, got nil", want)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce, "expected *connect.Error")
	assert.Equal(t, want, ce.Code(), "connect code")
}

// setupItemFixturesInStore creates workspace + tree + seed items in the store.
// Returns workspaceID.
func setupItemFixturesInStore(t *testing.T, store *mock.Store, userID string) string {
	t.Helper()
	ctx := context.Background()
	fixture := mock.CreateWorkspaceWithTreeFixture(t, ctx, store, userID)
	doc, _ := store.CreateDocument(ctx, fixture.Workspace.WorkspaceID, userID, "f.pdf", "application/pdf", 100)
	store.CreateProcessingJob(ctx, doc.DocumentID, fixture.Tree.TreeID, treev1.JobType_JOB_TYPE_PROCESS_DOCUMENT)
	return fixture.Workspace.WorkspaceID
}

// ── currentUser ──────────────────────────────────────────────────────────────

func TestCurrentUser_NoAuthInContext_ReturnsUnauthenticated(t *testing.T) {
	_, err := currentUser(context.Background())
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestCurrentUser_EmptyUserID_ReturnsUnauthenticated(t *testing.T) {
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "", Email: "x@y.com"})
	_, err := currentUser(ctx)
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestCurrentUser_ValidUser_ReturnsUser(t *testing.T) {
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "u1", Email: "u@example.com"})
	user, err := currentUser(ctx)
	require.NoError(t, err, "currentUser")
	assert.Equal(t, "u1", user.ID, "user.ID")
}

// ── authorizeWorkspace ────────────────────────────────────────────────────────

func TestAuthorizeWorkspace_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	store := mock.NewStore()
	err := authorizeWorkspace(context.Background(), store, "any_ws")
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestAuthorizeWorkspace_NotMember_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, context.Background(), store, "owner").Workspace.WorkspaceID
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeWorkspace(ctx, store, wsID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeWorkspace_Member_ReturnsNil(t *testing.T) {
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, context.Background(), store, "owner").Workspace.WorkspaceID
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	err := authorizeWorkspace(ctx, store, wsID)
	assert.NoError(t, err, "authorizeWorkspace")
}

// ── authorizeDocument ─────────────────────────────────────────────────────────

func TestAuthorizeDocument_DocumentNotFound_ReturnsNotFound(t *testing.T) {
	store := mock.NewStore()
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "u1", Email: "u@example.com"})

	err := authorizeDocument(ctx, store, store, "nonexistent_doc", "")
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestAuthorizeDocument_WrongWorkspace_ReturnsPermissionDenied(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	ws1ID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner").Workspace.WorkspaceID
	ws2ID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner2").Workspace.WorkspaceID
	doc, _ := store.CreateDocument(ctx, ws1ID, "owner", "f.pdf", "application/pdf", 100)
	authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	err := authorizeDocument(authedCtx, store, store, doc.DocumentID, ws2ID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeDocument_NotMember_ReturnsPermissionDenied(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner").Workspace.WorkspaceID
	doc, _ := store.CreateDocument(ctx, wsID, "owner", "f.pdf", "application/pdf", 100)
	authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeDocument(authedCtx, store, store, doc.DocumentID, "")
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeDocument_Member_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner").Workspace.WorkspaceID
	doc, _ := store.CreateDocument(ctx, wsID, "owner", "f.pdf", "application/pdf", 100)
	authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	err := authorizeDocument(authedCtx, store, store, doc.DocumentID, "")
	assert.NoError(t, err, "authorizeDocument")
}

// ── authorizeItem ─────────────────────────────────────────────────────────────

func TestAuthorizeItem_ItemNotFound_ReturnsNotFound(t *testing.T) {
	store := mock.NewStore()
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "u1", Email: "u@example.com"})

	err := authorizeItem(ctx, store, store, "nonexistent_item", "")
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestAuthorizeItem_ValidItem_AuthorizesViaWorkspace(t *testing.T) {
	store := mock.NewStore()
	wsID := setupItemFixturesInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	err := authorizeItem(ctx, store, store, "nd_root", wsID)
	assert.NoError(t, err, "authorizeItem")
}

func TestAuthorizeItem_NotMember_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	wsID := setupItemFixturesInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeItem(ctx, store, store, "nd_root", wsID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}
