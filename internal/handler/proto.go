package handler

import (
	"github.com/Keyhole-Koro/SynthifyShared/domain"
	graphv1 "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1"
)

func jobStatusToProto(status string) graphv1.JobLifecycleState {
	switch status {
	case string(domain.DocumentLifecycleCompleted):
		return graphv1.JobLifecycleState_JOB_LIFECYCLE_STATE_SUCCEEDED
	case string(domain.DocumentLifecycleFailed):
		return graphv1.JobLifecycleState_JOB_LIFECYCLE_STATE_FAILED
	case string(domain.DocumentLifecycleProcessing), "running":
		return graphv1.JobLifecycleState_JOB_LIFECYCLE_STATE_RUNNING
	default:
		return graphv1.JobLifecycleState_JOB_LIFECYCLE_STATE_QUEUED
	}
}
