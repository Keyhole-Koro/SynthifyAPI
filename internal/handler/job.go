package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
	"github.com/synthify/backend/api/internal/service"
)

type JobHandler struct {
	service    *service.JobService
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
}

func NewJobHandler(svc *service.JobService, workspaceRepo repository.WorkspaceRepository, documentRepo repository.DocumentRepository) *JobHandler {
	return &JobHandler{service: svc, workspaces: workspaceRepo, documents: documentRepo}
}

func (h *JobHandler) GetJobStatus(ctx context.Context, req *connect.Request[treev1.GetJobStatusRequest]) (*connect.Response[treev1.GetJobStatusResponse], error) {
	job, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&treev1.GetJobStatusResponse{Job: toProtoJob(job)}), nil
}

func (h *JobHandler) GetJobExecutionPlan(ctx context.Context, req *connect.Request[treev1.GetJobExecutionPlanRequest]) (*connect.Response[treev1.GetJobExecutionPlanResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	plan, err := h.service.GetExecutionPlan(req.Msg.GetJobId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.GetJobExecutionPlanResponse{
		Plan: &treev1.JobExecutionPlan{
			PlanId:    plan.PlanID,
			JobId:     plan.JobID,
			Status:    plan.Status,
			Summary:   plan.Summary,
			PlanJson:  plan.PlanJSON,
			CreatedBy: plan.CreatedBy,
			CreatedAt: plan.CreatedAt,
			UpdatedAt: plan.UpdatedAt,
		},
	}), nil
}

func (h *JobHandler) ListJobApprovalRequests(ctx context.Context, req *connect.Request[treev1.ListJobApprovalRequestsRequest]) (*connect.Response[treev1.ListJobApprovalRequestsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	requests, err := h.service.ListApprovalRequests(req.Msg.GetJobId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	res := connect.NewResponse(&treev1.ListJobApprovalRequestsResponse{})
	for _, request := range requests {
		res.Msg.Requests = append(res.Msg.Requests, toProtoApprovalRequest(request))
	}
	return res, nil
}

func (h *JobHandler) RequestJobApproval(ctx context.Context, req *connect.Request[treev1.RequestJobApprovalRequest]) (*connect.Response[treev1.RequestJobApprovalResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	approval, err := h.service.RequestApproval(req.Msg.GetJobId(), user.ID, req.Msg.GetReason())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.RequestJobApprovalResponse{Request: toProtoApprovalRequest(approval)}), nil
}

func (h *JobHandler) ApproveJobApproval(ctx context.Context, req *connect.Request[treev1.ApproveJobApprovalRequest]) (*connect.Response[treev1.ApproveJobApprovalResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetApprovalId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("approval_id is required"))
	}
	if err := h.service.ApproveApproval(req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.ApproveJobApprovalResponse{Status: "approved"}), nil
}

func (h *JobHandler) RejectJobApproval(ctx context.Context, req *connect.Request[treev1.RejectJobApprovalRequest]) (*connect.Response[treev1.RejectJobApprovalResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetApprovalId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("approval_id is required"))
	}
	if err := h.service.RejectApproval(req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID, req.Msg.GetReason()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&treev1.RejectJobApprovalResponse{Status: "rejected"}), nil
}

func (h *JobHandler) authorizeAndLoadJob(ctx context.Context, jobID string) (*domain.DocumentProcessingJob, error) {
	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}
	job, err := h.service.GetJob(jobID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, job.DocumentID, ""); err != nil {
		return nil, err
	}
	return job, nil
}

func toProtoJob(job *domain.DocumentProcessingJob) *treev1.Job {
	if job == nil {
		return nil
	}
	return &treev1.Job{
		JobId:        job.JobID,
		DocumentId:   job.DocumentID,
		Type:         job.JobType,
		Status:       job.Status,
		CreatedAt:    job.CreatedAt,
		CompletedAt:  job.UpdatedAt,
		ErrorMessage: job.ErrorMessage,
	}
}

func toProtoApprovalRequest(req *domain.JobApprovalRequest) *treev1.JobApprovalRequest {
	if req == nil {
		return nil
	}
	return &treev1.JobApprovalRequest{
		ApprovalId:          req.ApprovalID,
		JobId:               req.JobID,
		PlanId:              req.PlanID,
		Status:              req.Status,
		RequestedOperations: req.RequestedOperations,
		Reason:              req.Reason,
		RiskTier:            req.RiskTier,
		RequestedBy:         req.RequestedBy,
		ReviewedBy:          req.ReviewedBy,
		RequestedAt:         req.RequestedAt,
		ReviewedAt:          req.ReviewedAt,
	}
}
