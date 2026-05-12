package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func TestCreateDocumentRejectsOversizedFile(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	ws := store.CreateWorkspace(ctx, account.AccountID, "docs")
	svc := NewDocumentService(store, store, documentSourceURL, nil, nil, nil, nil)

	doc, uploadURL, err := svc.CreateDocument(ctx, ws.WorkspaceID, "owner", "huge.pdf", "application/pdf", account.MaxFileSizeBytes+1)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrFileTooLarge)
	assert.Nil(t, doc)
	assert.Empty(t, uploadURL)
}

func TestStartProcessingConfirmsUploadedObjectSize(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	ws := store.CreateWorkspace(ctx, account.AccountID, "docs")
	metadata := &fakeObjectMetadata{size: 128}
	svc := NewDocumentService(store, store, documentSourceURL, metadata, nil, nil, nil)
	doc, _, err := svc.CreateDocument(ctx, ws.WorkspaceID, "owner", "paper.pdf", "application/pdf", 128)
	require.NoError(t, err)

	job, err := svc.StartProcessing(ctx, ws.WorkspaceID, doc.DocumentID, false)

	require.NoError(t, err)
	require.NotNil(t, job)
	updated, err := store.GetAccount(ctx, account.AccountID)
	require.NoError(t, err)
	assert.Equal(t, int64(128), updated.StorageUsedBytes)
}

func TestStartProcessingRejectsSizeMismatch(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	ws := store.CreateWorkspace(ctx, account.AccountID, "docs")
	metadata := &fakeObjectMetadata{size: 256}
	svc := NewDocumentService(store, store, documentSourceURL, metadata, nil, nil, nil)
	doc, _, err := svc.CreateDocument(ctx, ws.WorkspaceID, "owner", "paper.pdf", "application/pdf", 128)
	require.NoError(t, err)

	job, err := svc.StartProcessing(ctx, ws.WorkspaceID, doc.DocumentID, false)

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrUploadSizeMismatch)
	assert.Nil(t, job)
	updated, err := store.GetAccount(ctx, account.AccountID)
	require.NoError(t, err)
	assert.Zero(t, updated.StorageUsedBytes)
}

func documentSourceURL(workspaceID, documentID string) string {
	return "https://storage.example/" + workspaceID + "/" + documentID
}

type fakeObjectMetadata struct {
	size int64
}

func (f *fakeObjectMetadata) GetObjectMetadata(ctx context.Context, workspaceID, documentID string) (*domain.ObjectMetadata, error) {
	return &domain.ObjectMetadata{Size: f.size, ContentType: "application/pdf"}, nil
}
