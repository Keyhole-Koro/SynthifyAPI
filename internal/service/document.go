package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/job/lifecycle"
	"github.com/synthify/backend/packages/shared/job/log"
	"github.com/synthify/backend/packages/shared/job/status"
	"github.com/synthify/backend/packages/shared/repository"
)

type WorkerDispatcher interface {
	GenerateExecutionPlan(ctx context.Context, req domain.ExecutePlanRequest) error
	ExecuteApprovedPlan(ctx context.Context, req domain.ExecutePlanRequest) error
}

type ObjectMetadataFetcher interface {
	GetObjectMetadata(ctx context.Context, workspaceID, documentID string) (*domain.ObjectMetadata, error)
}

type DocumentService struct {
	repo             repository.DocumentRepository
	tree             repository.TreeRepository
	sourceURLBuilder repository.DocumentSourceURLBuilder
	objectMetadata   ObjectMetadataFetcher
	dispatcher       WorkerDispatcher
	lifecycle        *joblifecycle.Service
	notifier         jobstatus.Notifier
	logger           applog.Logger
}

func NewDocumentService(
	repo repository.DocumentRepository,
	tree repository.TreeRepository,
	sourceURLBuilder repository.DocumentSourceURLBuilder,
	objectMetadata ObjectMetadataFetcher,
	dispatcher WorkerDispatcher,
	notifier jobstatus.Notifier,
	logger applog.Logger,
) *DocumentService {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &DocumentService{
		repo:             repo,
		tree:             tree,
		sourceURLBuilder: sourceURLBuilder,
		objectMetadata:   objectMetadata,
		dispatcher:       dispatcher,
		lifecycle:        joblifecycle.New(repo, notifier, logger),
		notifier:         notifier,
		logger:           logger,
	}
}

func (s *DocumentService) ListDocuments(ctx context.Context, workspaceID string) []*domain.Document {
	return s.repo.ListDocuments(ctx, workspaceID)
}

func (s *DocumentService) GetDocument(ctx context.Context, documentID string) (*domain.Document, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (s *DocumentService) CreateDocument(ctx context.Context, wsID, uploadedBy, filename, mimeType string, fileSize int64) (*domain.Document, string, error) {
	return s.repo.CreateDocument(ctx, wsID, uploadedBy, filename, mimeType, fileSize)
}

func (s *DocumentService) StartProcessing(ctx context.Context, wsID, documentID string, forceReprocess bool) (*domain.DocumentProcessingJob, error) {
	if !forceReprocess {
		if latest, err := s.repo.GetLatestProcessingJob(ctx, documentID); err == nil {
			switch latest.Status {
			case treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_SUCCEEDED,
				treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_RUNNING,
				treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_QUEUED:
				return latest, nil
			}
		}
	}

	jobType := treev1.JobType_JOB_TYPE_PROCESS_DOCUMENT
	if forceReprocess {
		jobType = treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT
	}
	return s.startProcessingJob(ctx, wsID, documentID, jobType, false)
}

func (s *DocumentService) ResumeProcessing(ctx context.Context, wsID, documentID string) (*domain.DocumentProcessingJob, error) {
	return s.startProcessingJob(ctx, wsID, documentID, treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT, true)
}

func (s *DocumentService) startProcessingJob(ctx context.Context, wsID, documentID string, jobType treev1.JobType, resumeExisting bool) (*domain.DocumentProcessingJob, error) {
	doc, err := s.repo.GetDocument(ctx, documentID)
	if err != nil {
		if resumeExisting {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if err := s.confirmUploadedObject(ctx, doc); err != nil {
		return nil, err
	}
	if resumeExisting {
		if latest, err := s.repo.GetLatestProcessingJob(ctx, documentID); err == nil {
			switch latest.Status {
			case treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_RUNNING,
				treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_QUEUED:
				return latest, nil
			}
		}
	}
	tree, err := s.tree.GetOrCreateTree(ctx, wsID)
	if err != nil {
		return nil, err
	}
	job := s.repo.CreateProcessingJob(ctx, documentID, wsID, jobType)
	if job == nil {
		return nil, domain.ErrNotFound
	}
	payload := documentJobPayload(job, documentID, wsID, tree.TreeID)
	s.lifecycle.NotifyQueued(ctx, payload)
	s.logJobQueued(ctx, job, wsID, documentID)
	if s.dispatcher != nil {
		dispatchReq := s.buildExecutePlanRequest(job, doc, wsID, tree.TreeID)
		if err := s.dispatcher.GenerateExecutionPlan(ctx, dispatchReq); err != nil {
			return s.handleDispatchFailure(ctx, job, payload, wsID, documentID, err, !resumeExisting), nil
		}
		if err := s.dispatcher.ExecuteApprovedPlan(ctx, dispatchReq); err != nil {
			if errors.Is(err, domain.ErrApprovalRequired) || errors.Is(err, domain.ErrPlanRejected) {
				if latest, err := s.repo.GetLatestProcessingJob(ctx, documentID); err == nil {
					return latest, nil
				}
				return job, nil
			}
			return s.handleDispatchFailure(ctx, job, payload, wsID, documentID, err, true), nil
		}
	}
	if latest, err := s.repo.GetLatestProcessingJob(ctx, documentID); err == nil {
		job = latest
	}
	return job, nil
}

func (s *DocumentService) confirmUploadedObject(ctx context.Context, doc *domain.Document) error {
	if s.objectMetadata == nil {
		return s.repo.ConfirmDocumentUpload(ctx, doc.DocumentID, doc.FileSize)
	}
	metadata, err := s.objectMetadata.GetObjectMetadata(ctx, doc.WorkspaceID, doc.DocumentID)
	if err != nil {
		return err
	}
	if metadata == nil {
		return domain.ErrUploadNotConfirmed
	}
	return s.repo.ConfirmDocumentUpload(ctx, doc.DocumentID, metadata.Size)
}

func (s *DocumentService) buildExecutePlanRequest(job *domain.DocumentProcessingJob, doc *domain.Document, wsID, treeID string) domain.ExecutePlanRequest {
	return domain.ExecutePlanRequest{
		JobID:       job.JobID,
		JobType:     job.JobType.String(),
		DocumentID:  doc.DocumentID,
		WorkspaceID: wsID,
		TreeID:      treeID,
		FileURI:     s.sourceURLBuilder(wsID, doc.DocumentID),
		Filename:    doc.Filename,
		MimeType:    doc.MimeType,
	}
}

func (s *DocumentService) logJobQueued(ctx context.Context, job *domain.DocumentProcessingJob, wsID, documentID string) {
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: wsID,
		DocumentID:  documentID,
		Level:       joblog.INFO,
		Event:       "job.queued",
		Message:     fmt.Sprintf("job queued: doc=%s type=%s", documentID, job.JobType),
		Detail:      map[string]any{"type": job.JobType.String()},
	})
}

func (s *DocumentService) handleDispatchFailure(ctx context.Context, job *domain.DocumentProcessingJob, payload jobstatus.Payload, wsID, documentID string, dispatchErr error, reloadLatest bool) *domain.DocumentProcessingJob {
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: wsID,
		DocumentID:  documentID,
		Level:       joblog.ERROR,
		Event:       "job.dispatch_failed",
		Message:     fmt.Sprintf("job dispatch failed: %v", dispatchErr),
		Detail:      map[string]any{"error": dispatchErr.Error()},
	})
	s.lifecycle.TryFail(ctx, payload, dispatchErr.Error())
	if reloadLatest {
		if latest, err := s.repo.GetLatestProcessingJob(ctx, documentID); err == nil {
			return latest
		}
	}
	return job
}

func documentJobPayload(job *domain.DocumentProcessingJob, documentID, wsID, treeID string) jobstatus.Payload {
	return jobstatus.Payload{
		JobID:       job.JobID,
		JobType:     job.JobType.String(),
		DocumentID:  documentID,
		WorkspaceID: wsID,
		TreeID:      treeID,
	}
}

func (s *DocumentService) GetLatestProcessingJob(ctx context.Context, documentID string) (*domain.DocumentProcessingJob, error) {
	job, err := s.repo.GetLatestProcessingJob(ctx, documentID)
	if err != nil {
		return nil, err
	}
	return job, nil
}
