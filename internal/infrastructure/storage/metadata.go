package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/synthify/backend/packages/shared/domain"
	sharedstorage "github.com/synthify/backend/packages/shared/storage"
)

type ObjectMetadataFetcher struct {
	baseURL string
	client  *http.Client
}

func NewObjectMetadataFetcher(baseURL string) *ObjectMetadataFetcher {
	return &ObjectMetadataFetcher{
		baseURL: baseURL,
		client:  http.DefaultClient,
	}
}

func (f *ObjectMetadataFetcher) GetObjectMetadata(ctx context.Context, workspaceID, documentID string) (*domain.ObjectMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sharedstorage.BuildDocumentObjectMetadataURL(f.baseURL, workspaceID, documentID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, domain.ErrUploadNotConfirmed
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("object metadata request failed status=%d: %s", resp.StatusCode, string(body))
	}
	var raw struct {
		Size        any    `json:"size"`
		ContentType string `json:"contentType"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	size, err := parseObjectSize(raw.Size)
	if err != nil {
		return nil, err
	}
	return &domain.ObjectMetadata{
		Size:        size,
		ContentType: raw.ContentType,
	}, nil
}

func parseObjectSize(value any) (int64, error) {
	switch typed := value.(type) {
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case float64:
		return int64(typed), nil
	default:
		return 0, errors.New("object metadata size is missing")
	}
}
