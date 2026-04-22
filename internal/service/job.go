package service

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type JobService struct {
	repo repository.DocumentRepository
}

func NewJobService(repo repository.DocumentRepository) *JobService {
	return &JobService{repo: repo}
}

func (s *JobService) GetJob(jobID string) (*domain.DocumentProcessingJob, error) {
	job, ok := s.repo.GetProcessingJob(jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return job, nil
}

func (s *JobService) GetExecutionPlan(jobID string) (*domain.JobExecutionPlan, error) {
	plan, ok := s.repo.GetJobExecutionPlan(jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return plan, nil
}

func (s *JobService) ListApprovalRequests(jobID string) ([]*domain.JobApprovalRequest, error) {
	requests, ok := s.repo.ListJobApprovalRequests(jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return requests, nil
}

func (s *JobService) RequestApproval(jobID, requestedBy, reason string) (*domain.JobApprovalRequest, error) {
	req, ok := s.repo.RequestJobApproval(jobID, requestedBy, reason)
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (s *JobService) ApproveApproval(jobID, approvalID, reviewedBy string) error {
	if !s.repo.ApproveJobApproval(jobID, approvalID, reviewedBy) {
		return ErrNotFound
	}
	return nil
}

func (s *JobService) RejectApproval(jobID, approvalID, reviewedBy, reason string) error {
	if !s.repo.RejectJobApproval(jobID, approvalID, reviewedBy, reason) {
		return ErrNotFound
	}
	return nil
}
