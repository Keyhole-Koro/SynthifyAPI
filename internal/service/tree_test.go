package service

import (
	"context"
	"testing"

	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

func TestGetTreeByWorkspace_ProcessedDocument_ReturnsItems(t *testing.T) {
	ctx := context.Background()
	mockStore := mock.NewStore()
	treeSvc := NewTreeService(mockStore)
	wsID := "ws-1"
	mockStore.CreateWorkspace(ctx, "acct-1", "Test")
	mockStore.CreateItem(ctx, wsID, "n1", "", "", "u1")

	items, err := treeSvc.GetTreeByWorkspace(ctx, wsID)
	if err != nil {
		t.Fatalf("GetTreeByWorkspace: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected items, got none")
	}
}

func TestFindPaths_ConnectedItems_ReturnsPaths(t *testing.T) {
	ctx := context.Background()
	mockStore := mock.NewStore()
	treeSvc := NewTreeService(mockStore)
	wsID := "ws-1"
	mockStore.CreateItem(ctx, wsID, "n1", "", "", "u1")
	mockStore.CreateItem(ctx, wsID, "n2", "", "item-n1", "u1")

	items, paths, err := treeSvc.FindPaths(ctx, wsID, "item-n2", "item-n1", 4, 3)
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
