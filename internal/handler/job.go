package handler

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	treev1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/tree/v1"
	"github.com/Keyhole-Koro/SynthifyShared/handlerutil"
	"github.com/Keyhole-Koro/SynthifyShared/mappers"
	"github.com/Keyhole-Koro/SynthifyShared/repository"
)

type JobHandler struct {
	repo       repository.DocumentRepository
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
}

func NewJobHandler(jobRepo repository.DocumentRepository, workspaceRepo repository.WorkspaceRepository, documentRepo repository.DocumentRepository) *JobHandler {
	return &JobHandler{repo: jobRepo, workspaces: workspaceRepo, documents: documentRepo}
}

func (h *JobHandler) GetJobStatus(ctx context.Context, req *connect.Request[treev1.GetJobStatusRequest]) (*connect.Response[treev1.GetJobStatusResponse], error) {
	job, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&treev1.GetJobStatusResponse{Job: mappers.ToProtoJob(job)}), nil
}

func (h *JobHandler) GetJobExecutionPlan(ctx context.Context, req *connect.Request[treev1.GetJobExecutionPlanRequest]) (*connect.Response[treev1.GetJobExecutionPlanResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	plan, ok := h.repo.GetJobExecutionPlan(ctx, req.Msg.GetJobId())
	if !ok {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	return connect.NewResponse(&treev1.GetJobExecutionPlanResponse{
		Plan: mappers.ToProtoExecutionPlan(plan),
	}), nil
}

func (h *JobHandler) ListJobApprovalRequests(ctx context.Context, req *connect.Request[treev1.ListJobApprovalRequestsRequest]) (*connect.Response[treev1.ListJobApprovalRequestsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	requests, ok := h.repo.ListJobApprovalRequests(ctx, req.Msg.GetJobId())
	if !ok {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	res := connect.NewResponse(&treev1.ListJobApprovalRequestsResponse{})
	for _, request := range requests {
		res.Msg.Requests = append(res.Msg.Requests, mappers.ToProtoApprovalRequest(request))
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
	approval, ok := h.repo.RequestJobApproval(ctx, req.Msg.GetJobId(), user.ID, req.Msg.GetReason())
	if !ok {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	return connect.NewResponse(&treev1.RequestJobApprovalResponse{Request: mappers.ToProtoApprovalRequest(approval)}), nil
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
	if !h.repo.ApproveJobApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID) {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
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
	if !h.repo.RejectJobApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID, req.Msg.GetReason()) {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	return connect.NewResponse(&treev1.RejectJobApprovalResponse{Status: "rejected"}), nil
}

func (h *JobHandler) ListJobMutationLogs(ctx context.Context, req *connect.Request[treev1.ListJobMutationLogsRequest]) (*connect.Response[treev1.ListJobMutationLogsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	logs, ok := h.repo.ListJobMutationLogs(ctx, req.Msg.GetJobId())
	if !ok {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	res := connect.NewResponse(&treev1.ListJobMutationLogsResponse{})
	for _, log := range logs {
		res.Msg.Logs = append(res.Msg.Logs, mappers.ToProtoMutationLog(log))
	}
	return res, nil
}

func (h *JobHandler) ListAllJobs(ctx context.Context, _ *connect.Request[treev1.ListAllJobsRequest]) (*connect.Response[treev1.ListAllJobsResponse], error) {
	// TODO: Add global admin authorization check here
	jobs, ok := h.repo.ListAllJobs(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeInternal, domain.ErrNotFound)
	}
	res := connect.NewResponse(&treev1.ListAllJobsResponse{})
	for _, job := range jobs {
		res.Msg.Jobs = append(res.Msg.Jobs, mappers.ToProtoJob(job))
	}
	return res, nil
}

func (h *JobHandler) authorizeAndLoadJob(ctx context.Context, jobID string) (*domain.DocumentProcessingJob, error) {
	if jobID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
	}
	job, ok := h.repo.GetProcessingJob(ctx, jobID)
	if !ok {
		return nil, handlerutil.ToConnectError(domain.ErrNotFound)
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, job.DocumentID, ""); err != nil {
		return nil, err
	}
	return job, nil
}
