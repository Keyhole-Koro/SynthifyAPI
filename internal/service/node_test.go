package service

import (
	"errors"
	"testing"

	"github.com/synthify/backend/internal/repository/mock"
)

// setupNodeFixtures creates an account, workspace, graph, and seed data so
// that nodes (nd_root, nd_tel, etc.) are available via the mock store.
// Returns the store and workspaceID.
func setupNodeFixtures(t *testing.T) (*mock.Store, string) {
	t.Helper()
	store := mock.NewStore()
	acct, err := store.GetOrCreateAccount("user_1")
	if err != nil {
		t.Fatalf("GetOrCreateAccount: %v", err)
	}
	ws := store.CreateWorkspace(acct.AccountID, "test-workspace")
	if ws == nil {
		t.Fatal("CreateWorkspace returned nil")
	}
	g, err := store.GetOrCreateGraph(ws.WorkspaceID)
	if err != nil {
		t.Fatalf("GetOrCreateGraph: %v", err)
	}
	doc, _ := store.CreateDocument(ws.WorkspaceID, "user_1", "test.pdf", "application/pdf", 1024)
	store.CreateProcessingJob(doc.DocumentID, g.GraphID, "process_document")
	return store, ws.WorkspaceID
}

func TestGetGraphEntityDetail_ExistingNode_ReturnsNodeAndEdges(t *testing.T) {
	store, _ := setupNodeFixtures(t)
	svc := NewNodeService(store, store)

	node, edges, err := svc.GetGraphEntityDetail("nd_root")
	if err != nil {
		t.Fatalf("GetGraphEntityDetail: unexpected error: %v", err)
	}
	if node.NodeID != "nd_root" {
		t.Errorf("node.NodeID = %q, want nd_root", node.NodeID)
	}
	if len(edges) == 0 {
		t.Error("expected edges for nd_root, got none")
	}
}

func TestGetGraphEntityDetail_UnknownNode_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	svc := NewNodeService(store, store)

	_, _, err := svc.GetGraphEntityDetail("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetGraphEntityDetail unknown node: err = %v, want ErrNotFound", err)
	}
}

func TestApproveAlias_UnknownNodes_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	acct, err := store.GetOrCreateAccount("u1")
	if err != nil {
		t.Fatalf("GetOrCreateAccount: %v", err)
	}
	ws := store.CreateWorkspace(acct.AccountID, "ws")
	svc := NewNodeService(store, store)

	err = svc.ApproveAlias(ws.WorkspaceID, "nonexistent_canonical", "nonexistent_alias")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("ApproveAlias unknown nodes: err = %v, want ErrNotFound", err)
	}
}

func TestApproveAlias_ValidNodes_ReturnsNil(t *testing.T) {
	store, wsID := setupNodeFixtures(t)
	svc := NewNodeService(store, store)

	err := svc.ApproveAlias(wsID, "nd_root", "nd_tel")
	if err != nil {
		t.Errorf("ApproveAlias valid nodes: unexpected error: %v", err)
	}
}

func TestRejectAlias_UnknownNodes_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	acct, err := store.GetOrCreateAccount("u1")
	if err != nil {
		t.Fatalf("GetOrCreateAccount: %v", err)
	}
	ws := store.CreateWorkspace(acct.AccountID, "ws")
	svc := NewNodeService(store, store)

	err = svc.RejectAlias(ws.WorkspaceID, "nonexistent_canonical", "nonexistent_alias")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("RejectAlias unknown nodes: err = %v, want ErrNotFound", err)
	}
}

func TestRejectAlias_ValidNodes_ReturnsNil(t *testing.T) {
	store, wsID := setupNodeFixtures(t)
	svc := NewNodeService(store, store)

	// Approve first so there is something to reject.
	_ = svc.ApproveAlias(wsID, "nd_root", "nd_tel")

	err := svc.RejectAlias(wsID, "nd_root", "nd_tel")
	if err != nil {
		t.Errorf("RejectAlias valid nodes: unexpected error: %v", err)
	}
}
