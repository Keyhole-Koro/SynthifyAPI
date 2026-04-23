package service

import (
	"testing"

	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

func TestGetTreeByWorkspace_ProcessedDocument_ReturnsItems(t *testing.T) {
	mockStore := mock.NewStore()
	treeSvc := NewTreeService(mockStore)
	wsID := "ws-1"
	mockStore.CreateWorkspace("acct-1", "Test")
	mockStore.CreateItem(wsID, "n1", "", "", "u1")

	items, err := treeSvc.GetTreeByWorkspace(wsID)
	if err != nil {
		t.Fatalf("GetTreeByWorkspace: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected items, got none")
	}
}

func TestFindPaths_ConnectedItems_ReturnsPaths(t *testing.T) {
	mockStore := mock.NewStore()
	treeSvc := NewTreeService(mockStore)
	wsID := "ws-1"
	mockStore.CreateItem(wsID, "n1", "", "", "u1")
	mockStore.CreateItem(wsID, "n2", "", "item-n1", "u1")

	items, paths, err := treeSvc.FindPaths(wsID, "item-n2", "item-n1", 4, 3)
	if err != nil {
		t.Fatalf("FindPaths: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected items in result")
	}
	if len(paths) == 0 {
		t.Error("expected paths between items")
	}
}
