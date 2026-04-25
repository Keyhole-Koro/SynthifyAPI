package service

import (
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
	wsID := "ws-1"
	repo.CreateWorkspace("acct-1", "Test")
	repo.CreateItem(wsID, "root", "root desc", "", "system")

	item, err := svc.GetTreeEntityDetail("item-root")
	if err != nil {
		t.Fatalf("GetTreeEntityDetail: %v", err)
	}
	if item == nil {
		t.Fatal("expected item, got nil")
	}
}

func TestApproveAlias_CallsRepo(t *testing.T) {
	wsID := "ws-1"
	err := svc.ApproveAlias(wsID, "n1", "n2")
	if err != nil {
		t.Errorf("ApproveAlias: %v", err)
	}
}
