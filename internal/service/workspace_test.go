package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func createWorkspaceForUser(t *testing.T, store *mock.Store, userID string) string {
	t.Helper()
	ctx := context.Background()
	acct, err := store.GetOrCreateAccount(ctx, userID)
	require.NoError(t, err, "GetOrCreateAccount")
	ws := store.CreateWorkspace(ctx, acct.AccountID, "test-workspace")
	require.NotNil(t, ws, "CreateWorkspace returned nil")
	return ws.WorkspaceID
}

func TestGetWorkspace_NonMember_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace(ctx, wsID, "stranger")
	assert.ErrorIs(t, err, domain.ErrNotFound, "GetWorkspace non-member")
}

func TestGetWorkspace_Member_ReturnsWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := createWorkspaceForUser(t, store, "owner")
	svc := NewWorkspaceService(store, store)

	got, err := svc.GetWorkspace(ctx, wsID, "owner")
	require.NoError(t, err, "GetWorkspace")
	assert.Equal(t, wsID, got.WorkspaceID, "workspace ID")
}

func TestGetWorkspace_UnknownID_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	_, err := svc.GetWorkspace(ctx, "nonexistent_ws", "anyone")
	assert.ErrorIs(t, err, domain.ErrNotFound, "GetWorkspace unknown ID")
}

func TestCreateWorkspace_NewUser_CreatesWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store)

	ws, err := svc.CreateWorkspace(ctx, "my-workspace", "new_user")
	require.NoError(t, err, "CreateWorkspace")
	require.NotNil(t, ws, "CreateWorkspace returned nil workspace")
	assert.Equal(t, "my-workspace", ws.Name, "workspace.Name")
}
