package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	connect "connectrpc.com/connect"
	"github.com/synthify/backend/packages/shared/applog"
	"github.com/synthify/backend/packages/shared/domain"
	treev1 "github.com/synthify/backend/packages/shared/gen/synthify/tree/v1"
	"github.com/synthify/backend/packages/shared/handlerutil"
	"github.com/synthify/backend/packages/shared/joblifecycle"
	"github.com/synthify/backend/packages/shared/joblog"
	"github.com/synthify/backend/packages/shared/mappers"
	"github.com/synthify/backend/packages/shared/repository"
)

type JobHandler struct {
	repo       repository.DocumentRepository
	workspaces repository.WorkspaceRepository
	documents  repository.DocumentRepository
	lifecycle  *joblifecycle.Service
}

func NewJobHandler(jobRepo repository.DocumentRepository, workspaceRepo repository.WorkspaceRepository, documentRepo repository.DocumentRepository, logger applog.Logger) *JobHandler {
	if logger == nil {
		logger = applog.NoopLogger{}
	}
	return &JobHandler{
		repo:       jobRepo,
		workspaces: workspaceRepo,
		documents:  documentRepo,
		lifecycle:  joblifecycle.New(jobRepo, nil, logger),
	}
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
	plan, err := h.repo.GetJobExecutionPlan(ctx, req.Msg.GetJobId())
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
	requests, err := h.repo.ListJobApprovalRequests(ctx, req.Msg.GetJobId())
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
	job, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	approval, err := h.lifecycle.RequestApproval(ctx, req.Msg.GetJobId(), user.ID, req.Msg.GetReason())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: job.WorkspaceID,
		DocumentID:  job.DocumentID,
		Level:       joblog.INFO,
		Event:       "approval.requested",
		Message:     fmt.Sprintf("approval requested by=%s reason=%q", user.ID, req.Msg.GetReason()),
		Detail:      map[string]any{"by": user.ID, "reason": req.Msg.GetReason()},
	})
	return connect.NewResponse(&treev1.RequestJobApprovalResponse{Request: mappers.ToProtoApprovalRequest(approval)}), nil
}

func (h *JobHandler) ApproveJobApproval(ctx context.Context, req *connect.Request[treev1.ApproveJobApprovalRequest]) (*connect.Response[treev1.ApproveJobApprovalResponse], error) {
	job, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetApprovalId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("approval_id is required"))
	}
	if err := h.lifecycle.ApproveApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID); err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: job.WorkspaceID,
		DocumentID:  job.DocumentID,
		Level:       joblog.INFO,
		Event:       "approval.approved",
		Message:     fmt.Sprintf("approval approved by=%s approval_id=%s", user.ID, req.Msg.GetApprovalId()),
		Detail:      map[string]any{"by": user.ID, "approval_id": req.Msg.GetApprovalId()},
	})
	return connect.NewResponse(&treev1.ApproveJobApprovalResponse{Status: "approved"}), nil
}

func (h *JobHandler) RejectJobApproval(ctx context.Context, req *connect.Request[treev1.RejectJobApprovalRequest]) (*connect.Response[treev1.RejectJobApprovalResponse], error) {
	job, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, err
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetApprovalId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("approval_id is required"))
	}
	if err := h.lifecycle.RejectApproval(ctx, req.Msg.GetJobId(), req.Msg.GetApprovalId(), user.ID, req.Msg.GetReason()); err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	joblog.FromContext(ctx).Log(ctx, joblog.Event{
		JobID:       job.JobID,
		WorkspaceID: job.WorkspaceID,
		DocumentID:  job.DocumentID,
		Level:       joblog.WARN,
		Event:       "approval.rejected",
		Message:     fmt.Sprintf("approval rejected by=%s approval_id=%s reason=%q", user.ID, req.Msg.GetApprovalId(), req.Msg.GetReason()),
		Detail:      map[string]any{"by": user.ID, "approval_id": req.Msg.GetApprovalId(), "reason": req.Msg.GetReason()},
	})
	return connect.NewResponse(&treev1.RejectJobApprovalResponse{Status: "rejected"}), nil
}

func (h *JobHandler) ListJobMutationLogs(ctx context.Context, req *connect.Request[treev1.ListJobMutationLogsRequest]) (*connect.Response[treev1.ListJobMutationLogsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	logs, err := h.repo.ListJobMutationLogs(ctx, req.Msg.GetJobId())
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	res := connect.NewResponse(&treev1.ListJobMutationLogsResponse{})
	for _, log := range logs {
		res.Msg.Logs = append(res.Msg.Logs, mappers.ToProtoMutationLog(log))
	}
	return res, nil
}

func (h *JobHandler) ListJobLogs(ctx context.Context, req *connect.Request[treev1.ListJobLogsRequest]) (*connect.Response[treev1.ListJobLogsResponse], error) {
	if _, err := h.authorizeAndLoadJob(ctx, req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	limit := int(req.Msg.GetPageSize())
	if limit <= 0 {
		limit = 500
	}
	logs, nextToken, err := h.repo.ListJobLogs(ctx, req.Msg.GetJobId(), req.Msg.GetPageToken(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list job logs: %w", err))
	}
	res := connect.NewResponse(&treev1.ListJobLogsResponse{
		NextPageToken: nextToken,
	})
	for _, l := range logs {
		res.Msg.Logs = append(res.Msg.Logs, mappers.ToProtoJobLog(l))
	}
	return res, nil
}

func (h *JobHandler) SearchJobLogs(ctx context.Context, req *connect.Request[treev1.SearchJobLogsRequest]) (*connect.Response[treev1.SearchJobLogsResponse], error) {
	filter := domain.JobLogSearchFilter{
		Query:         req.Msg.GetQuery(),
		WorkspaceID:   req.Msg.GetWorkspaceId(),
		DocumentID:    req.Msg.GetDocumentId(),
		JobID:         req.Msg.GetJobId(),
		Levels:        req.Msg.GetLevels(),
		Events:        req.Msg.GetEvents(),
		FromTimestamp: req.Msg.GetFromTimestamp(),
		ToTimestamp:   req.Msg.GetToTimestamp(),
		PageToken:     req.Msg.GetPageToken(),
		Limit:         int(req.Msg.GetPageSize()),
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}
	if err := h.authorizeLogSearch(ctx, filter); err != nil {
		return nil, err
	}
	logs, nextToken, err := h.repo.SearchJobLogs(ctx, filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to search job logs"))
	}
	res := connect.NewResponse(&treev1.SearchJobLogsResponse{
		NextPageToken: nextToken,
	})
	for _, l := range logs {
		res.Msg.Logs = append(res.Msg.Logs, mappers.ToProtoJobLog(l))
	}
	return res, nil
}

func (h *JobHandler) ListRelatedJobLogs(ctx context.Context, req *connect.Request[treev1.ListRelatedJobLogsRequest]) (*connect.Response[treev1.ListRelatedJobLogsResponse], error) {
	scope := domain.RelatedLogScope(strings.ToLower(req.Msg.GetScope().String()))
	// Handle enum string mapping if needed, but RelatedLogScopeJob etc match the lowercase enum names in domain
	switch req.Msg.GetScope() {
	case treev1.RelatedLogScope_RELATED_LOG_SCOPE_JOB:
		scope = domain.RelatedLogScopeJob
	case treev1.RelatedLogScope_RELATED_LOG_SCOPE_DOCUMENT:
		scope = domain.RelatedLogScopeDocument
	case treev1.RelatedLogScope_RELATED_LOG_SCOPE_WORKSPACE:
		scope = domain.RelatedLogScopeWorkspace
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported scope"))
	}

	if err := h.authorizeRelatedLogSearch(ctx, scope, req.Msg.GetWorkspaceId(), req.Msg.GetDocumentId(), req.Msg.GetJobId()); err != nil {
		return nil, err
	}
	limit := int(req.Msg.GetPageSize())
	if limit <= 0 {
		limit = 100
	}
	groups, nextToken, err := h.repo.ListRelatedJobLogs(ctx, scope, req.Msg.GetWorkspaceId(), req.Msg.GetDocumentId(), req.Msg.GetJobId(), req.Msg.GetPageToken(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list related job logs"))
	}
	res := connect.NewResponse(&treev1.ListRelatedJobLogsResponse{
		NextPageToken: nextToken,
	})
	for _, g := range groups {
		res.Msg.Groups = append(res.Msg.Groups, mappers.ToProtoJobLogGroup(g))
	}
	return res, nil
}

func (h *JobHandler) ListAllJobs(ctx context.Context, _ *connect.Request[treev1.ListAllJobsRequest]) (*connect.Response[treev1.ListAllJobsResponse], error) {
	// TODO: Add global admin authorization check here
	jobs, err := h.repo.ListAllJobs(ctx)
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
	job, err := h.repo.GetProcessingJob(ctx, jobID)
	if err != nil {
		return nil, handlerutil.ToConnectError(err)
	}
	if err := authorizeDocument(ctx, h.workspaces, h.documents, job.DocumentID, ""); err != nil {
		return nil, err
	}
	return job, nil
}

func (h *JobHandler) authorizeLogSearch(ctx context.Context, filter domain.JobLogSearchFilter) error {
	switch {
	case filter.JobID != "":
		_, err := h.authorizeAndLoadJob(ctx, filter.JobID)
		return err
	case filter.DocumentID != "":
		return authorizeDocument(ctx, h.workspaces, h.documents, filter.DocumentID, filter.WorkspaceID)
	case filter.WorkspaceID != "":
		return authorizeWorkspace(ctx, h.workspaces, filter.WorkspaceID)
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id, document_id, or job_id is required"))
	}
}

func (h *JobHandler) authorizeRelatedLogSearch(ctx context.Context, scope domain.RelatedLogScope, workspaceID, documentID, jobID string) error {
	switch scope {
	case domain.RelatedLogScopeJob:
		if jobID == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("job_id is required"))
		}
		_, err := h.authorizeAndLoadJob(ctx, jobID)
		return err
	case domain.RelatedLogScopeDocument:
		if documentID == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("document_id is required"))
		}
		return authorizeDocument(ctx, h.workspaces, h.documents, documentID, workspaceID)
	case domain.RelatedLogScopeWorkspace:
		if workspaceID == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("workspace_id is required"))
		}
		return authorizeWorkspace(ctx, h.workspaces, workspaceID)
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("scope must be job, document, or workspace"))
	}
}
