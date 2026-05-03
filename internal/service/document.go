package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/joblog"
	"github.com/synthify/backend/packages/shared/jobstatus"
	"github.com/synthify/backend/packages/shared/repository"
)

type WorkerDispatcher interface {
	GenerateExecutionPlan(ctx context.Context, req domain.ExecutePlanRequest) error
	ExecuteApprovedPlan(ctx context.Context, req domain.ExecutePlanRequest) error
}

type DocumentService struct {
	repo             repository.DocumentRepository
	tree             repository.TreeRepository
	sourceURLBuilder repository.DocumentSourceURLBuilder
	dispatcher       WorkerDispatcher
	notifier         jobstatus.Notifier
}

func NewDocumentService(
	repo repository.DocumentRepository,
	tree repository.TreeRepository,
	sourceURLBuilder repository.DocumentSourceURLBuilder,
	dispatcher WorkerDispatcher,
	notifier jobstatus.Notifier,
) *DocumentService {
	return &DocumentService{
		repo:             repo,
		tree:             tree,
		sourceURLBuilder: sourceURLBuilder,
		dispatcher:       dispatcher,
		notifier:         notifier,
	}
}

func (s *DocumentService) ListDocuments(ctx context.Context, workspaceID string) []*domain.Document {
	return s.repo.ListDocuments(ctx, workspaceID)
}

func (s *DocumentService) GetDocument(ctx context.Context, documentID string) (*domain.Document, error) {
	doc, ok := s.repo.GetDocument(ctx, documentID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return doc, nil
}

func (s *DocumentService) CreateDocument(ctx context.Context, wsID, uploadedBy, filename, mimeType string, fileSize int64) (*domain.Document, string) {
	return s.repo.CreateDocument(ctx, wsID, uploadedBy, filename, mimeType, fileSize)
}

func (s *DocumentService) StartProcessing(ctx context.Context, wsID, documentID string, forceReprocess bool) (*domain.DocumentProcessingJob, error) {
	doc, ok := s.repo.GetDocument(ctx, documentID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	tree, err := s.tree.GetOrCreateTree(ctx, wsID)
	if err != nil {
		return nil, err
	}
	jobType := treev1.JobType_JOB_TYPE_PROCESS_DOCUMENT
	if forceReprocess {
		jobType = treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT
	}
	job := s.repo.CreateProcessingJob(ctx, documentID, tree.TreeID, jobType)
	if job == nil {
		return nil, domain.ErrNotFound
	}
	if s.notifier != nil {
		s.notifier.Queued(ctx, jobstatus.Payload{
			JobID:       job.JobID,
			JobType:     job.JobType.String(),
			DocumentID:  documentID,
			WorkspaceID: wsID,
			TreeID:      tree.TreeID,
		})
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: wsID,
		DocumentID:  documentID,
		Level:       joblog.INFO,
		Event:       "job.queued",
		Message:     fmt.Sprintf("job queued: doc=%s type=%s", documentID, jobType),
		Detail:      map[string]any{"type": jobType.String()},
	})
	if s.dispatcher != nil {
		dispatchReq := domain.ExecutePlanRequest{
			JobID:       job.JobID,
			JobType:     job.JobType.String(),
			DocumentID:  documentID,
			WorkspaceID: wsID,
			TreeID:      tree.TreeID,
			FileURI:     s.sourceURLBuilder(wsID, doc.DocumentID),
			Filename:    doc.Filename,
			MimeType:    doc.MimeType,
		}
		if err := s.dispatcher.GenerateExecutionPlan(ctx, dispatchReq); err != nil {
			joblog.FromContext(ctx).Log(ctx, joblog.Event{
				JobID:       job.JobID,
				WorkspaceID: wsID,
				DocumentID:  documentID,
				Level:       joblog.ERROR,
				Event:       "job.dispatch_failed",
				Message:     fmt.Sprintf("job dispatch failed: %v", err),
				Detail:      map[string]any{"error": err.Error()},
			})
			s.repo.FailProcessingJob(ctx, job.JobID, err.Error())
			if s.notifier != nil {
				s.notifier.Failed(ctx, jobstatus.Payload{
					JobID:       job.JobID,
					JobType:     job.JobType.String(),
					DocumentID:  documentID,
					WorkspaceID: wsID,
					TreeID:      tree.TreeID,
				}, err.Error())
			}
			return job, nil
		}
		if err := s.dispatcher.ExecuteApprovedPlan(ctx, dispatchReq); err != nil {
			if errors.Is(err, domain.ErrApprovalRequired) || errors.Is(err, domain.ErrPlanRejected) {
				if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
					return latest, nil
				}
				return job, nil
			}
			joblog.FromContext(ctx).Log(ctx, joblog.Event{
				JobID:       job.JobID,
				WorkspaceID: wsID,
				DocumentID:  documentID,
				Level:       joblog.ERROR,
				Event:       "job.dispatch_failed",
				Message:     fmt.Sprintf("job dispatch failed: %v", err),
				Detail:      map[string]any{"error": err.Error()},
			})
			s.repo.FailProcessingJob(ctx, job.JobID, err.Error())
			if s.notifier != nil {
				s.notifier.Failed(ctx, jobstatus.Payload{
					JobID:       job.JobID,
					JobType:     job.JobType.String(),
					DocumentID:  documentID,
					WorkspaceID: wsID,
					TreeID:      tree.TreeID,
				}, err.Error())
			}
			if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
				return latest, nil
			}
			return job, nil
		}
	}
	if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
		job = latest
	}
	return job, nil
}

func (s *DocumentService) ResumeProcessing(ctx context.Context, wsID, documentID string) (*domain.DocumentProcessingJob, error) {
	doc, ok := s.repo.GetDocument(ctx, documentID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
		switch latest.Status {
		case treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_RUNNING,
			treev1.JobLifecycleState_JOB_LIFECYCLE_STATE_QUEUED:
			return latest, nil
		}
	}

	tree, err := s.tree.GetOrCreateTree(ctx, wsID)
	if err != nil {
		return nil, err
	}
	job := s.repo.CreateProcessingJob(ctx, documentID, tree.TreeID, treev1.JobType_JOB_TYPE_REPROCESS_DOCUMENT)
	if job == nil {
		return nil, domain.ErrNotFound
	}
	if s.notifier != nil {
		s.notifier.Queued(ctx, jobstatus.Payload{
			JobID:       job.JobID,
			JobType:     job.JobType.String(),
			DocumentID:  documentID,
			WorkspaceID: wsID,
			TreeID:      tree.TreeID,
		})
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: wsID,
		DocumentID:  documentID,
		Level:       joblog.INFO,
		Event:       "job.queued",
		Message:     fmt.Sprintf("job queued: doc=%s type=%s", documentID, job.JobType),
		Detail:      map[string]any{"type": job.JobType.String()},
	})
	if s.dispatcher != nil {
		dispatchReq := domain.ExecutePlanRequest{
			JobID:       job.JobID,
			JobType:     job.JobType.String(),
			DocumentID:  documentID,
			WorkspaceID: wsID,
			TreeID:      tree.TreeID,
			FileURI:     s.sourceURLBuilder(wsID, doc.DocumentID),
			Filename:    doc.Filename,
			MimeType:    doc.MimeType,
		}
		if err := s.dispatcher.GenerateExecutionPlan(ctx, dispatchReq); err != nil {
			joblog.FromContext(ctx).Log(ctx, joblog.Event{
				JobID:       job.JobID,
				WorkspaceID: wsID,
				DocumentID:  documentID,
				Level:       joblog.ERROR,
				Event:       "job.dispatch_failed",
				Message:     fmt.Sprintf("job dispatch failed: %v", err),
				Detail:      map[string]any{"error": err.Error()},
			})
			s.repo.FailProcessingJob(ctx, job.JobID, err.Error())
			if s.notifier != nil {
				s.notifier.Failed(ctx, jobstatus.Payload{
					JobID:       job.JobID,
					JobType:     job.JobType.String(),
					DocumentID:  documentID,
					WorkspaceID: wsID,
					TreeID:      tree.TreeID,
				}, err.Error())
			}
			return job, nil
		}
		if err := s.dispatcher.ExecuteApprovedPlan(ctx, dispatchReq); err != nil {
			if errors.Is(err, domain.ErrApprovalRequired) || errors.Is(err, domain.ErrPlanRejected) {
				if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
					return latest, nil
				}
				return job, nil
			}
			joblog.FromContext(ctx).Log(ctx, joblog.Event{
				JobID:       job.JobID,
				WorkspaceID: wsID,
				DocumentID:  documentID,
				Level:       joblog.ERROR,
				Event:       "job.dispatch_failed",
				Message:     fmt.Sprintf("job dispatch failed: %v", err),
				Detail:      map[string]any{"error": err.Error()},
			})
			s.repo.FailProcessingJob(ctx, job.JobID, err.Error())
			if s.notifier != nil {
				s.notifier.Failed(ctx, jobstatus.Payload{
					JobID:       job.JobID,
					JobType:     job.JobType.String(),
					DocumentID:  documentID,
					WorkspaceID: wsID,
					TreeID:      tree.TreeID,
				}, err.Error())
			}
			if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
				return latest, nil
			}
			return job, nil
		}
	}
	if latest, ok := s.repo.GetLatestProcessingJob(ctx, documentID); ok {
		job = latest
	}
	return job, nil
}

func (s *DocumentService) GetLatestProcessingJob(ctx context.Context, documentID string) (*domain.DocumentProcessingJob, error) {
	job, ok := s.repo.GetLatestProcessingJob(ctx, documentID)
	if !ok {
		return nil, domain.ErrNotFound
	}
	return job, nil
}
