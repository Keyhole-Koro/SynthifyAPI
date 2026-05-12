package handler

import (
	"context"
	"errors"
	"fmt"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/apps/api/internal/service"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/repository"
	"github.com/synthify/backend/packages/shared/transport/connect"
	"github.com/synthify/backend/packages/shared/transport/connect/mappers"
)

type DocumentHandler struct {
	service          *service.DocumentService
	workspaces       repository.WorkspaceRepository
	documents        repository.DocumentRepository
	uploadURLBuilder repository.DocumentUploadURLBuilder
}

func NewDocumentHandler(
	svc *service.DocumentService,
	workspaceRepo repository.WorkspaceRepository,
	documentRepo repository.DocumentRepository,
	uploadURLBuilder repository.DocumentUploadURLBuilder,
) *DocumentHandler {
	return &DocumentHandler{
		service:          svc,
		workspaces:       workspaceRepo,
		documents:        documentRepo,
		uploadURLBuilder: uploadURLBuilder,
	}
}

func (h *DocumentHandler) ListDocuments(ctx context.Context, req *connect.Request[treev1.ListDocumentsRequest]) (*connect.Response[treev1.ListDocumentsResponse], error) {
	if req.Msg.GetWorkspaceId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	docs := h.service.ListDocuments(ctx, req.Msg.GetWorkspaceId())
	res := connect.NewResponse(&treev1.ListDocumentsResponse{})
	for _, doc := range docs {
		res.Msg.Documents = append(res.Msg.Documents, mappers.ToProtoDocument(doc))
	}
	return res, nil
}

func (h *DocumentHandler) GetDocument(ctx context.Context, req *connect.Request[treev1.GetDocumentRequest]) (*connect.Response[treev1.GetDocumentResponse], error) {
	if req.Msg.GetDocumentId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("document_id is required"))
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, req.Msg.GetDocumentId(), ""); err != nil {
		return nil, err
	}
	doc, err := h.service.GetDocument(ctx, req.Msg.GetDocumentId())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.GetDocumentResponse{Document: mappers.ToProtoDocument(doc)}), nil
}

func (h *DocumentHandler) CreateDocument(ctx context.Context, req *connect.Request[treev1.CreateDocumentRequest]) (*connect.Response[treev1.CreateDocumentResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetFilename() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id and filename are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	doc, uploadURL, err := h.service.CreateDocument(ctx, req.Msg.GetWorkspaceId(), user.ID, req.Msg.GetFilename(), req.Msg.GetMimeType(), req.Msg.GetFileSize())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.CreateDocumentResponse{
		Document:          mappers.ToProtoDocument(doc),
		UploadUrl:         uploadURL,
		UploadMethod:      "POST",
		UploadContentType: req.Msg.GetMimeType(),
	}), nil
}

func (h *DocumentHandler) GetUploadURL(ctx context.Context, req *connect.Request[treev1.GetUploadURLRequest]) (*connect.Response[treev1.GetUploadURLResponse], error) {
	if req.Msg.GetWorkspaceId() == "" || req.Msg.GetFilename() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id and filename are required"))
	}
	if err := authorizeWorkspace(ctx, h.workspaces, req.Msg.GetWorkspaceId()); err != nil {
		return nil, err
	}
	token := fmt.Sprintf("upload-%s", req.Msg.GetFilename())
	// GetUploadURL uses a special tokenized path. If that also needs to be shared,
	// extend the Generator. For now, keep the Generator as the base and wrap it as needed.
	uploadURL := h.uploadURLBuilder(req.Msg.GetWorkspaceId(), token+"/"+req.Msg.GetFilename())
	return connect.NewResponse(&treev1.GetUploadURLResponse{
		UploadUrl:   uploadURL,
		UploadToken: token,
		ExpiresAt:   "",
	}), nil
}

func (h *DocumentHandler) StartProcessing(ctx context.Context, req *connect.Request[treev1.StartProcessingRequest]) (*connect.Response[treev1.StartProcessingResponse], error) {
	if req.Msg.GetDocumentId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("document_id is required"))
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, req.Msg.GetDocumentId(), ""); err != nil {
		return nil, err
	}
	doc, err := h.documents.GetDocument(ctx, req.Msg.GetDocumentId())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	job, err := h.service.StartProcessing(ctx, doc.WorkspaceID, req.Msg.GetDocumentId(), req.Msg.GetForceReprocess())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.StartProcessingResponse{
		DocumentId: req.Msg.GetDocumentId(),
		Job: &treev1.Job{
			JobId:      job.JobID,
			DocumentId: job.DocumentID,
			Type:       treev1.JobType_JOB_TYPE_PROCESS_DOCUMENT,
			Status:     job.Status,
		},
	}), nil
}

func (h *DocumentHandler) ResumeProcessing(ctx context.Context, req *connect.Request[treev1.ResumeProcessingRequest]) (*connect.Response[treev1.ResumeProcessingResponse], error) {
	if req.Msg.GetDocumentId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("document_id is required"))
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, req.Msg.GetDocumentId(), ""); err != nil {
		return nil, err
	}
	doc, err := h.service.GetDocument(ctx, req.Msg.GetDocumentId())
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	job, err := h.service.ResumeProcessing(ctx, doc.WorkspaceID, doc.DocumentID)
	if err != nil {
		return nil, connectutil.ToError(err)
	}
	return connect.NewResponse(&treev1.ResumeProcessingResponse{
		DocumentId: doc.DocumentID,
		Job: &treev1.Job{
			JobId:      job.JobID,
			DocumentId: doc.DocumentID,
			Type:       treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT,
			Status:     job.Status,
		},
	}), nil
}
