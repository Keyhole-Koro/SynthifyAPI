package service

import (
	"context"

	"github.com/Keyhole-Koro/SynthifyShared/domain"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type JobService struct {
	repo repository.DocumentRepository
}

func NewJobService(repo repository.DocumentRepository) *JobService {
	return &JobService{repo: repo}
}

func (s *JobService) GetJob(ctx context.Context, jobID string) (*domain.DocumentProcessingJob, error) {
	job, ok := s.repo.GetProcessingJob(ctx, jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return job, nil
}

func (s *JobService) GetExecutionPlan(ctx context.Context, jobID string) (*domain.JobExecutionPlan, error) {
	plan, ok := s.repo.GetJobExecutionPlan(ctx, jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return plan, nil
}

func (s *JobService) ListApprovalRequests(ctx context.Context, jobID string) ([]*domain.JobApprovalRequest, error) {
	requests, ok := s.repo.ListJobApprovalRequests(ctx, jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return requests, nil
}

func (s *JobService) ListMutationLogs(ctx context.Context, jobID string) ([]*domain.JobMutationLog, error) {
	logs, ok := s.repo.ListJobMutationLogs(ctx, jobID)
	if !ok {
		return nil, ErrNotFound
	}
	return logs, nil
}

func (s *JobService) ListAllJobs(ctx context.Context) ([]*domain.DocumentProcessingJob, error) {
	jobs, ok := s.repo.ListAllJobs(ctx)
	if !ok {
		return nil, ErrNotFound
	}
	return jobs, nil
}

func (s *JobService) RequestApproval(ctx context.Context, jobID, requestedBy, reason string) (*domain.JobApprovalRequest, error) {
	req, ok := s.repo.RequestJobApproval(ctx, jobID, requestedBy, reason)
	if !ok {
		return nil, ErrNotFound
	}
	return req, nil
}

func (s *JobService) ApproveApproval(ctx context.Context, jobID, approvalID, reviewedBy string) error {
	if !s.repo.ApproveJobApproval(ctx, jobID, approvalID, reviewedBy) {
		return ErrNotFound
	}
	return nil
}

func (s *JobService) RejectApproval(ctx context.Context, jobID, approvalID, reviewedBy, reason string) error {
	if !s.repo.RejectJobApproval(ctx, jobID, approvalID, reviewedBy, reason) {
		return ErrNotFound
	}
	return nil
}
