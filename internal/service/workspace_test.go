package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository/mock"
)

func createWorkspaceForUser(t *testing.T, store *mock.Store, userID string) string {
	t.Helper()
	ctx := context.Background()
	acct, err := store.GetOrCreateAccount(ctx, userID)
	if err != nil {
		t.Fatalf("GetOrCreateAccount: %v", err)
	}
	ws := store.CreateWorkspace(ctx, acct.AccountID, "test-workspace")
	if ws == nil {
		t.Fatal("CreateWorkspace returned nil")
	}
	return ws.WorkspaceID
}

func TestGetWorkspace_NonMember_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace(ctx, wsID, "stranger")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetWorkspace non-member: err = %v, want ErrNotFound", err)
	}
}

func TestGetWorkspace_Member_ReturnsWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	got, err := svc.GetWorkspace(ctx, wsID, "owner")
	if err != nil {
		t.Fatalf("GetWorkspace: unexpected error: %v", err)
	}
	if got.WorkspaceID != wsID {
		t.Errorf("workspace ID = %q, want %q", got.WorkspaceID, wsID)
	}
}

func TestGetWorkspace_UnknownID_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace(ctx, "nonexistent_ws", "anyone")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetWorkspace unknown ID: err = %v, want ErrNotFound", err)
	}
}

func TestCreateWorkspace_NewUser_CreatesWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	ws, err := svc.CreateWorkspace(ctx, "my-workspace", "new_user")
	if err != nil {
		t.Fatalf("CreateWorkspace: unexpected error: %v", err)
	}
	if ws == nil {
		t.Fatal("CreateWorkspace returned nil workspace")
	}
	if ws.Name != "my-workspace" {
		t.Errorf("workspace.Name = %q, want my-workspace", ws.Name)
	}
}
