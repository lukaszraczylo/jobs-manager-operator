package controllers

import (
	corev1 "k8s.io/api/core/v1"
)

func (cp *connPackage) checkGroupsStatus() {
	groupsTotal := len(cp.mj.Spec.Groups)
	totalJobs := 0
	completedJobs := 0
	didAnyJobAbort := false
	changePresent := false
	groupsCompleted := 0

	// Check if all groups have completed and set ManagedJob status to "succeeded" if so.
	if cp.mj.Spec.Status != ExecutionStatusSucceeded && groupsTotal > 0 {
		for _, group := range cp.mj.Spec.Groups {
			groupJobsTotal := len(group.Jobs)
			groupJobsCompleted := 0

			for _, job := range group.Jobs {
				if job.Status == ExecutionStatusSucceeded {
					groupJobsCompleted++
					completedJobs++
				}
				if job.Status == ExecutionStatusFailed || job.Status == ExecutionStatusAborted {
					didAnyJobAbort = true
				}
				totalJobs++
			}

			if groupJobsTotal == groupJobsCompleted {
				// All the jobs in the group are completed.
				if group.Status != ExecutionStatusSucceeded {
					group.Status = ExecutionStatusSucceeded
					changePresent = true
				}
				groupsCompleted++
			}
		}
	}

	if groupsTotal == groupsCompleted && cp.mj.Spec.Status != ExecutionStatusSucceeded && cp.mj.Status != ExecutionStatusSucceeded {
		cp.mj.Spec.Status = ExecutionStatusSucceeded
		changePresent = true
		cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Completed", "All jobs completed")
	}

	// Update status if any job aborted.
	if didAnyJobAbort && cp.mj.Spec.Status != ExecutionStatusFailed {
		cp.mj.Spec.Status = ExecutionStatusFailed
		changePresent = true
		cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Aborted", "One of the jobs aborted")
	}

	// Update status to "running" if not already set.
	// if cp.mj.Spec.Status != ExecutionStatusRunning && cp.mj.Spec.Status != ExecutionStatusSucceeded {
	// 	cp.mj.Spec.Status = ExecutionStatusRunning
	// 	cp.mj.Status = ExecutionStatusRunning
	// 	changePresent = true
	// }

	// Check if the ManagedJob status has changed.
	statusChanged := cp.mj.Spec.Status != cp.mj.Status

	// Update status and send event if it has changed.
	if statusChanged || changePresent {
		cp.updateCRDStatusDirectly()
		// cp.r.Client.Status().Update(cp.ctx, cp.mj)
	}
}
