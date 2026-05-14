package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
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
	assert.Equal(t, ws.WorkspaceID, job.WorkspaceID)
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

func TestStartProcessingRespectsForceReprocess(t *testing.T) {
	ctx := context.Background()
	store := mock.NewStore()
	account, err := store.GetOrCreateAccount(ctx, "owner")
	require.NoError(t, err)
	ws := store.CreateWorkspace(ctx, account.AccountID, "docs")
	metadata := &fakeObjectMetadata{size: 128}
	svc := NewDocumentService(store, store, documentSourceURL, metadata, nil, nil, nil)
	doc, _, err := svc.CreateDocument(ctx, ws.WorkspaceID, "owner", "paper.pdf", "application/pdf", 128)
	require.NoError(t, err)

	// First time
	job1, err := svc.StartProcessing(ctx, ws.WorkspaceID, doc.DocumentID, false)
	require.NoError(t, err)
	require.NotNil(t, job1)
	job1.Status = treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_SUCCEEDED

	// Second time without force - should return job1
	job2, err := svc.StartProcessing(ctx, ws.WorkspaceID, doc.DocumentID, false)
	require.NoError(t, err)
	assert.Equal(t, job1.JobID, job2.JobID, "should return existing completed job")

	// Third time with force - should return a new job
	job3, err := svc.StartProcessing(ctx, ws.WorkspaceID, doc.DocumentID, true)
	require.NoError(t, err)
	assert.NotEqual(t, job1.JobID, job3.JobID, "should create new job when forced")
	assert.Equal(t, treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT, job3.JobType)
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
