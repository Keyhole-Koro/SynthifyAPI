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
	return connect.NewResponse(&treev1.GetJobStatusResponse{Job: mappers.ToProtoJob(job)}), nil
}

func (h *JobHandler) GetJobExecutionPlan(ctx context.Context, req *connect.Request[treev1.GetJobExecutionPlanRequest]) (*connect.Response[treev1.GetJobExecutionPlanResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	plan, err := h.service.GetExecutionPlan(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.GetJobExecutionPlanResponse{
		Plan: mappers.ToProtoExecutionPlan(plan),
	}), nil
}

func (h *JobHandler) ListJobApprovalRequests(ctx context.Context, req *connect.Request[treev1.ListJobApprovalRequestsRequest]) (*connect.Response[treev1.ListJobApprovalRequestsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	requests, err := h.service.ListApprovalRequests(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
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
	approval, err := h.service.RequestApproval(ctx, req.Msg.GetJobId(), user.ID, req.Msg.GetReason())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
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
	if err := h.service.ApproveApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID); err != nil {
		return nil, handlerutil.ToConnectError(err)
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
	if err := h.service.RejectApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID, req.Msg.GetReason()); err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	return connect.NewResponse(&treev1.RejectJobApprovalResponse{Status: "rejected"}), nil
}

func (h *JobHandler) ListJobMutationLogs(ctx context.Context, req *connect.Request[treev1.ListJobMutationLogsRequest]) (*connect.Response[treev1.ListJobMutationLogsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	logs, err := h.service.ListMutationLogs(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	res := connect.NewResponse(&treev1.ListJobMutationLogsResponse{})
	for _, log := range logs {
		res.Msg.Logs = append(res.Msg.Logs, mappers.ToProtoMutationLog(log))
	}
	return res, nil
}

func (h *JobHandler) ListAllJobs(ctx context.Context, _ *connect.Request[treev1.ListAllJobsRequest]) (*connect.Response[treev1.ListAllJobsResponse], error) {
	// TODO: Add global admin authorization check here
	jobs, err := h.service.ListAllJobs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
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
	job, err := h.service.GetJob(ctx, jobID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, job.DocumentID, ""); err != nil {
		return nil, err
	}
	return job, nil
}
