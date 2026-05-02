package service

import (
	"context"
	"testing"

	"github.com/synthify/backend/packages/shared/repository/mock"
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

func TestCreateItem_CreatesItem(t *testing.T) {
	ctx := context.Background()
	wsID := "ws-1"
	repo.CreateWorkspace(ctx, "acct-1", "Test")

	item, err := svc.CreateItem(ctx, wsID, "root", "root desc", "", "system")
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if item == nil {
		t.Fatal("expected item, got nil")
	}
}
