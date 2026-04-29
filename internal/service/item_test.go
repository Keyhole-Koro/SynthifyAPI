package service

import (
	"context"
	"testing"

	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

var (
	repo *mock.Store
	svc  *ItemService
)

func TestMain(m *testing.M) {
	repo = mock.NewStore()
	svc = NewItemService(repo, repo)
	m.Run()
}

func TestGetTreeEntityDetail_ExistingItem_ReturnsItem(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-1"
	repo.CreateWorkspace(ctx, "acct-1", "Test")
	repo.CreateItem(ctx, wsID, "root", "root desc", "", "system")

	item, err := svc.GetTreeEntityDetail(ctx, "item-root")
	if err != nil {
		t.Fatalf("GetTreeEntityDetail: %v", err)
	}
	if item == nil {
		t.Fatal("expected item, got nil")
	}
}

func TestApproveAlias_CallsRepo(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-1"
	err := svc.ApproveAlias(ctx, wsID, "n1", "n2")
	if err != nil {
		t.Errorf("ApproveAlias: %v", err)
	}
}
