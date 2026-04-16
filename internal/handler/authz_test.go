package handler

import (
	"context"
	"errors"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

// assertConnectCode fails the test if err is nil or does not carry the expected connect code.
func assertConnectCode(t *testing.T, err error, want connect.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %v, got nil", want)
	}
	var ce *connect.Error
	if !errors.As(err, &ce) {
		t.Fatalf("expected *connect.Error, got %T: %v", err, err)
	}
	if ce.Code() != want {
		t.Errorf("connect code = %v, want %v", ce.Code(), want)
	}
}

// setupWorkspaceInStore creates an account and workspace for a given userID.
func setupWorkspaceInStore(t *testing.T, store *mock.Store, userID string) string {
	t.Helper()
	acct, err := store.GetOrCreateAccount(userID)
	if err != nil {
		t.Fatalf("GetOrCreateAccount: %v", err)
	}
	ws := store.CreateWorkspace(acct.AccountID, "test-workspace")
	if ws == nil {
		t.Fatal("CreateWorkspace returned nil")
	}
	return ws.WorkspaceID
}

// setupNodeFixturesInStore creates workspace + graph + seed nodes in the store.
// Returns workspaceID.
func setupNodeFixturesInStore(t *testing.T, store *mock.Store, userID string) string {
	t.Helper()
	wsID := setupWorkspaceInStore(t, store, userID)
	g, err := store.GetOrCreateGraph(wsID)
	if err != nil {
		t.Fatalf("GetOrCreateGraph: %v", err)
	}
	doc, _ := store.CreateDocument(wsID, userID, "f.pdf", "application/pdf", 100)
	store.CreateProcessingJob(doc.DocumentID, g.GraphID, "process_document")
	return wsID
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
	if err != nil {
		t.Fatalf("currentUser: unexpected error: %v", err)
	}
	if user.ID != "u1" {
		t.Errorf("user.ID = %q, want u1", user.ID)
	}
}

// ── authorizeWorkspace ────────────────────────────────────────────────────────

func TestAuthorizeWorkspace_Unauthenticated_ReturnsUnauthenticated(t *testing.T) {
	store := mock.NewStore()
	err := authorizeWorkspace(context.Background(), store, "any_ws")
	assertConnectCode(t, err, connect.CodeUnauthenticated)
}

func TestAuthorizeWorkspace_NotMember_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	wsID := setupWorkspaceInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeWorkspace(ctx, store, wsID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeWorkspace_Member_ReturnsNil(t *testing.T) {
	store := mock.NewStore()
	wsID := setupWorkspaceInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	if err := authorizeWorkspace(ctx, store, wsID); err != nil {
		t.Errorf("authorizeWorkspace: unexpected error: %v", err)
	}
}

// ── authorizeDocument ─────────────────────────────────────────────────────────

func TestAuthorizeDocument_DocumentNotFound_ReturnsNotFound(t *testing.T) {
	store := mock.NewStore()
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "u1", Email: "u@example.com"})

	err := authorizeDocument(ctx, store, store, "nonexistent_doc", "")
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestAuthorizeDocument_WrongWorkspace_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	ws1ID := setupWorkspaceInStore(t, store, "owner")
	ws2ID := setupWorkspaceInStore(t, store, "owner2")
	doc, _ := store.CreateDocument(ws1ID, "owner", "f.pdf", "application/pdf", 100)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	err := authorizeDocument(ctx, store, store, doc.DocumentID, ws2ID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeDocument_NotMember_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	wsID := setupWorkspaceInStore(t, store, "owner")
	doc, _ := store.CreateDocument(wsID, "owner", "f.pdf", "application/pdf", 100)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeDocument(ctx, store, store, doc.DocumentID, "")
	assertConnectCode(t, err, connect.CodePermissionDenied)
}

func TestAuthorizeDocument_Member_ReturnsNil(t *testing.T) {
	store := mock.NewStore()
	wsID := setupWorkspaceInStore(t, store, "owner")
	doc, _ := store.CreateDocument(wsID, "owner", "f.pdf", "application/pdf", 100)
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	if err := authorizeDocument(ctx, store, store, doc.DocumentID, ""); err != nil {
		t.Errorf("authorizeDocument: unexpected error: %v", err)
	}
}

// ── authorizeNode ─────────────────────────────────────────────────────────────

func TestAuthorizeNode_NodeNotFound_ReturnsNotFound(t *testing.T) {
	store := mock.NewStore()
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "u1", Email: "u@example.com"})

	err := authorizeNode(ctx, store, store, "nonexistent_node", "")
	assertConnectCode(t, err, connect.CodeNotFound)
}

func TestAuthorizeNode_ValidNode_AuthorizesViaWorkspace(t *testing.T) {
	store := mock.NewStore()
	wsID := setupNodeFixturesInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "owner", Email: "o@example.com"})

	if err := authorizeNode(ctx, store, store, "nd_root", wsID); err != nil {
		t.Errorf("authorizeNode: unexpected error: %v", err)
	}
}

func TestAuthorizeNode_NotMember_ReturnsPermissionDenied(t *testing.T) {
	store := mock.NewStore()
	wsID := setupNodeFixturesInStore(t, store, "owner")
	ctx := middleware.ContextWithUser(context.Background(), middleware.AuthUser{ID: "stranger", Email: "s@example.com"})

	err := authorizeNode(ctx, store, store, "nd_root", wsID)
	assertConnectCode(t, err, connect.CodePermissionDenied)
}
