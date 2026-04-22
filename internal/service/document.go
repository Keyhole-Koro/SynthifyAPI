package service

import (
	"context"
	"errors"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/worker/pkg/worker"
)

type DocumentService struct {
	repo               repository.DocumentRepository
	graph              repository.GraphRepository
	sourceURLGenerator repository.UploadURLGenerator
	dispatcher         worker.Dispatcher
	notifier           jobstatus.Notifier
}

func NewDocumentService(
	repo repository.DocumentRepository,
	graph repository.GraphRepository,
	sourceURLGenerator repository.UploadURLGenerator,
	dispatcher worker.Dispatcher,
	notifier jobstatus.Notifier,
) *DocumentService {
	return &DocumentService{
		repo:               repo,
		graph:              graph,
		sourceURLGenerator: sourceURLGenerator,
		dispatcher:         dispatcher,
		notifier:           notifier,
	}
}

func (s *DocumentService) ListDocuments(workspaceID string) []*domain.Document {
	return s.repo.ListDocuments(workspaceID)
}

func (s *DocumentService) GetDocument(documentID string) (*domain.Document, error) {
	doc, ok := s.repo.GetDocument(documentID)
	if !ok {
		return nil, ErrNotFound
	}
	return doc, nil
}

func (s *DocumentService) CreateDocument(wsID, uploadedBy, filename, mimeType string, fileSize int64) (*domain.Document, string) {
	return s.repo.CreateDocument(wsID, uploadedBy, filename, mimeType, fileSize)
}

func (s *DocumentService) StartProcessing(wsID, documentID string, forceReprocess bool) (*domain.DocumentProcessingJob, error) {
	_ = forceReprocess
	doc, ok := s.repo.GetDocument(documentID)
	if !ok {
		return nil, ErrNotFound
	}
	graph, err := s.graph.GetOrCreateGraph(wsID)
	if err != nil {
		return nil, err
	}
	job := s.repo.CreateProcessingJob(documentID, graph.GraphID, "process_document")
	if job == nil {
		return nil, ErrNotFound
	}
	if s.notifier != nil {
		s.notifier.Queued(context.Background(), jobstatus.Payload{
			JobID:       job.JobID,
			JobType:     job.JobType,
			DocumentID:  documentID,
			WorkspaceID: wsID,
			GraphID:     graph.GraphID,
		})
	}
	if s.dispatcher != nil {
		dispatchReq := worker.ExecutePlanRequest{
			JobID:       job.JobID,
			JobType:     job.JobType,
			DocumentID:  documentID,
			WorkspaceID: wsID,
			GraphID:     graph.GraphID,
			FileURI:     s.sourceURLGenerator(wsID, doc.DocumentID),
			Filename:    doc.Filename,
			MimeType:    doc.MimeType,
		}
		if err := s.dispatcher.GenerateExecutionPlan(context.Background(), dispatchReq); err != nil {
			s.repo.FailProcessingJob(job.JobID, err.Error())
			return job, nil
		}
		if err := s.dispatcher.ExecuteApprovedPlan(context.Background(), dispatchReq); err != nil {
			if errors.Is(err, worker.ErrApprovalRequired) || errors.Is(err, worker.ErrPlanRejected) {
				if latest, ok := s.repo.GetLatestProcessingJob(documentID); ok {
					return latest, nil
				}
				return job, nil
			}
			s.repo.FailProcessingJob(job.JobID, err.Error())
			if s.notifier != nil {
				s.notifier.Failed(context.Background(), jobstatus.Payload{
					JobID:       job.JobID,
					JobType:     job.JobType,
					DocumentID:  documentID,
					WorkspaceID: wsID,
					GraphID:     graph.GraphID,
				}, err.Error())
			}
			if latest, ok := s.repo.GetLatestProcessingJob(documentID); ok {
				return latest, nil
			}
			return job, nil
		}
	}
	if latest, ok := s.repo.GetLatestProcessingJob(documentID); ok {
		job = latest
	}
	return job, nil
}

func (s *DocumentService) GetLatestProcessingJob(documentID string) (*domain.DocumentProcessingJob, error) {
	job, ok := s.repo.GetLatestProcessingJob(documentID)
	if !ok {
		return nil, ErrNotFound
	}
	return job, nil
}
