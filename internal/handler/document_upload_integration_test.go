package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/synthify/backend/apps/api/internal/service"
	"github.com/synthify/backend/packages/shared/app"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/middleware"
	"github.com/synthify/backend/packages/shared/repository/mock"
)

func TestDocumentUpload_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	gcsBaseURL := "http://localhost:4443"
	// Check if fake-gcs-server is running
	resp, err := http.Get(gcsBaseURL + "/storage/v1/b")
	if err != nil {
		t.Skipf("fake-gcs-server not running at %s, skipping integration test", gcsBaseURL)
	}
	resp.Body.Close()

	// Ensure bucket exists
	bucket := "synthify-uploads"
	createBucketURL := fmt.Sprintf("%s/storage/v1/b?project=synthify-local", gcsBaseURL)
	createBucketBody := fmt.Sprintf(`{"name":"%s"}`, bucket)
	http.Post(createBucketURL, "application/json", strings.NewReader(createBucketBody))

	ctx := context.Background()
	store := mock.NewStore()
	fixture := mock.CreateUserWorkspaceFixture(t, ctx, store, "owner")

	// Real URL builder
	uploadURLBuilder := app.NewDocumentUploadURLBuilder(gcsBaseURL)
	store.SetUploadURLBuilder(uploadURLBuilder)

	svc := service.NewDocumentService(store, store, nil, nil, nil, nil, nil)
	handler := NewDocumentHandler(svc, store, store, uploadURLBuilder)

	authedCtx := middleware.ContextWithUser(ctx, middleware.AuthUser{ID: "owner", Email: "owner@example.com"})

	t.Run("upload via issued URL using POST", func(t *testing.T) {
		// 1. Get Upload URL
		createResp, err := handler.CreateDocument(authedCtx, connect.NewRequest(&treev1.CreateDocumentRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Filename:    "test-post.txt",
			MimeType:    "text/plain",
			FileSize:    12,
		}))
		require.NoError(t, err)
		uploadURL := createResp.Msg.GetUploadUrl()
		assert.NotEmpty(t, uploadURL)
		assert.Equal(t, "POST", createResp.Msg.GetUploadMethod())

		// 2. Perform Upload
		content := "hello world!"
		req, err := http.NewRequest(createResp.Msg.GetUploadMethod(), uploadURL, strings.NewReader(content))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "text/plain")

		uploadResp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer uploadResp.Body.Close()

		if uploadResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(uploadResp.Body)
			t.Fatalf("POST upload failed with status %d: %s", uploadResp.StatusCode, string(body))
		}

		// 3. Verify object exists
		docID := createResp.Msg.GetDocument().GetDocumentId()
		verifyURL := fmt.Sprintf("%s/storage/v1/b/%s/o/%s%%2F%s?alt=media", gcsBaseURL, bucket, fixture.Workspace.WorkspaceID, docID)
		verifyResp, err := http.Get(verifyURL)
		require.NoError(t, err)
		defer verifyResp.Body.Close()
		assert.Equal(t, http.StatusOK, verifyResp.StatusCode)

		downloaded, err := io.ReadAll(verifyResp.Body)
		require.NoError(t, err)
		assert.Equal(t, content, string(downloaded))
	})

	t.Run("upload via issued URL using PUT (check if emulator supports it)", func(t *testing.T) {
		// 1. Get Upload URL
		createResp, err := handler.CreateDocument(authedCtx, connect.NewRequest(&treev1.CreateDocumentRequest{
			WorkspaceId: fixture.Workspace.WorkspaceID,
			Filename:    "test-put.txt",
			MimeType:    "text/plain",
			FileSize:    12,
		}))
		require.NoError(t, err)
		uploadURL := createResp.Msg.GetUploadUrl()

		// 2. Perform Upload with PUT
		content := "hello world PUT!"
		req, err := http.NewRequest(http.MethodPut, uploadURL, strings.NewReader(content))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "text/plain")

		uploadResp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer uploadResp.Body.Close()

		if uploadResp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(uploadResp.Body)
			t.Logf("PUT upload returned status %d: %s", uploadResp.StatusCode, string(body))
		} else {
			t.Log("PUT upload succeeded")
		}
	})
}
