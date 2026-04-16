package service

import (
	"errors"
	"testing"

	"github.com/synthify/backend/internal/repository/mock"
)

// createWorkspaceForUser is a helper that creates an account and workspace for userID.
func createWorkspaceForUser(t *testing.T, store *mock.Store, userID string) string {
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

func TestGetWorkspace_NonMember_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace(wsID, "stranger")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetWorkspace non-member: err = %v, want ErrNotFound", err)
	}
}

func TestGetWorkspace_Member_ReturnsWorkspace(t *testing.T) {
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	got, err := svc.GetWorkspace(wsID, "owner")
	if err != nil {
		t.Fatalf("GetWorkspace: unexpected error: %v", err)
	}
	if got.WorkspaceID != wsID {
		t.Errorf("workspace ID = %q, want %q", got.WorkspaceID, wsID)
	}
}

func TestGetWorkspace_UnknownID_ReturnsErrNotFound(t *testing.T) {
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace("nonexistent_ws", "anyone")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetWorkspace unknown ID: err = %v, want ErrNotFound", err)
	}
}

func TestCreateWorkspace_NewUser_CreatesWorkspace(t *testing.T) {
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	ws, err := svc.CreateWorkspace("my-workspace", "new_user")
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

func TestListWorkspaces_ReturnsOnlyUserWorkspaces(t *testing.T) {
	store := mock.NewStore()
	createWorkspaceForUser(t, store, "user_a")
	createWorkspaceForUser(t, store, "user_b")
	svc := NewWorkspaceService(store, store)

	got := svc.ListWorkspaces("user_a")
	if len(got) != 1 {
		t.Errorf("ListWorkspaces user_a: got %d workspaces, want 1", len(got))
	}
}
