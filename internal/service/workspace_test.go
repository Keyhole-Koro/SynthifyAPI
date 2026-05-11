package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func TestGetWorkspace_NonMember_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner").Workspace.WorkspaceID
	svc := NewWorkspaceService(store, store, nil)

	_, err := svc.GetWorkspace(ctx, wsID, "stranger")
	assert.ErrorIs(t, err, domain.ErrNotFound, "GetWorkspace non-member")
}

func TestGetWorkspace_Member_ReturnsWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	wsID := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner").Workspace.WorkspaceID
	svc := NewWorkspaceService(store, store, nil)

	got, err := svc.GetWorkspace(ctx, wsID, "owner")
	require.NoError(t, err, "GetWorkspace")
	assert.Equal(t, wsID, got.WorkspaceID, "workspace ID")
}

func TestGetWorkspace_UnknownID_ReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store, nil)

	_, err := svc.GetWorkspace(ctx, "nonexistent_ws", "anyone")
	assert.ErrorIs(t, err, domain.ErrNotFound, "GetWorkspace unknown ID")
}

func TestCreateWorkspace_NewUser_CreatesWorkspace(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	svc := NewWorkspaceService(store, store, nil)

	ws, err := svc.CreateWorkspace(ctx, "my-workspace", "new_user")
	require.NoError(t, err, "CreateWorkspace")
	require.NotNil(t, ws, "CreateWorkspace returned nil workspace")
	assert.Equal(t, "my-workspace", ws.Name, "workspace.Name")
}
