package service

import (
	"errors"
	"testing"

	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

// setupGraphFixtures creates an account, workspace, graph, and seed data.
// Returns the store, workspaceID, and graphID.
func setupGraphFixtures(t *testing.T) (*mock.Store, string, string) {
	t.Helper()
	store := mock.NewStore()
	acct, err := store.GetOrCreateAccount("u1")
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
	doc, _ := store.CreateDocument(ws.WorkspaceID, "u1", "test.pdf", "application/pdf", 1024)
	store.CreateProcessingJob(doc.DocumentID, g.GraphID, "process_document")
	return store, ws.WorkspaceID, g.GraphID
}

func TestGetGraphByWorkspace_WorkspaceNotFound_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	svc := NewGraphService(store)

	_, _, err := svc.GetGraphByWorkspace("nonexistent_ws")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetGraphByWorkspace missing ws: err = %v, want ErrNotFound", err)
	}
}

func TestGetGraphByWorkspace_ProcessedDocument_ReturnsNodesAndEdges(t *testing.T) {
	store, wsID, _ := setupGraphFixtures(t)
	svc := NewGraphService(store)

	nodes, edges, err := svc.GetGraphByWorkspace(wsID)
	if err != nil {
		t.Fatalf("GetGraphByWorkspace: unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Error("expected nodes, got none")
	}
	if len(edges) == 0 {
		t.Error("expected edges, got none")
	}
}

func TestFindPaths_GraphNotFound_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	svc := NewGraphService(store)

	_, _, _, err := svc.FindPaths("nonexistent_graph", "n1", "n2", 4, 3)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("FindPaths missing graph: err = %v, want ErrNotFound", err)
	}
}

func TestFindPaths_ConnectedNodes_ReturnsPaths(t *testing.T) {
	store, _, graphID := setupGraphFixtures(t)
	svc := NewGraphService(store)

	// nd_root → nd_tel → nd_cv is a known path in the seed data.
	nodes, edges, paths, err := svc.FindPaths(graphID, "nd_root", "nd_cv", 4, 3)
	if err != nil {
		t.Fatalf("FindPaths: unexpected error: %v", err)
	}
	if len(nodes) == 0 {
		t.Error("expected nodes in result")
	}
	if len(edges) == 0 {
		t.Error("expected edges in result")
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one path from nd_root to nd_cv")
	}
	if paths[0].NodeIDs[0] != "nd_root" {
		t.Errorf("path start = %q, want nd_root", paths[0].NodeIDs[0])
	}
	last := paths[0].NodeIDs[len(paths[0].NodeIDs)-1]
	if last != "nd_cv" {
		t.Errorf("path end = %q, want nd_cv", last)
	}
}
