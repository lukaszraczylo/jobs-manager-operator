package controllers

import (
	"strings"

	"github.com/lukaszraczylo/pandati"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (cp *connPackage) compileParameters(params ...jobsmanagerv1beta1.ManagedJobParameters) jobsmanagerv1beta1.ManagedJobParameters {
	cparams := jobsmanagerv1beta1.ManagedJobParameters{}
	for _, params := range params {
		if !pandati.IsZero(params) {
			if params.FromEnv != nil {
				cparams.FromEnv = append(cparams.FromEnv, params.FromEnv...)
			}
			if params.Env != nil {
				cparams.Env = append(cparams.Env, params.Env...)
			}
			if params.Volumes != nil {
				cparams.Volumes = append(cparams.Volumes, params.Volumes...)
			}
			if params.VolumeMounts != nil {
				cparams.VolumeMounts = append(cparams.VolumeMounts, params.VolumeMounts...)
			}
			if params.ServiceAccount != "" {
				cparams.ServiceAccount = params.ServiceAccount
			}
			if params.RestartPolicy != "" {
				cparams.RestartPolicy = params.RestartPolicy
			}
			if params.ImagePullSecrets != nil {
				cparams.ImagePullSecrets = append(cparams.ImagePullSecrets, params.ImagePullSecrets...)
			}
			if params.ImagePullPolicy != "" {
				cparams.ImagePullPolicy = params.ImagePullPolicy
			}
			if params.Labels != nil {
				cparams.Labels = make(map[string]string)
				for k, v := range params.Labels {
					cparams.Labels[k] = v
				}
			}
			if params.Annotations != nil {
				cparams.Annotations = make(map[string]string)
				for k, v := range params.Annotations {
					cparams.Annotations[k] = v
				}
			}
			if params.Resources != nil {
				cparams.Resources = params.Resources
			}
		}
	}
	return cparams
}

// updateDependentJobs updates the status of all dependencies that reference the completed job.
// Uses the pre-built jobDepMap for O(1) lookup instead of iterating through all jobs.
func (cp *connPackage) updateDependentJobs(completedJob string, jobStatus string) {
	if deps, exists := cp.jobDepMap[completedJob]; exists {
		for _, dep := range deps {
			if dep.Status != jobStatus {
				dep.Status = jobStatus
			}
		}
	}
}

// updateDependentGroups updates the status of all dependencies that reference the completed group.
// Uses the pre-built groupDepMap for O(1) lookup instead of iterating through all groups.
func (cp *connPackage) updateDependentGroups(completedGroup string, jobStatus string) {
	if deps, exists := cp.groupDepMap[completedGroup]; exists {
		for _, dep := range deps {
			if dep.Status != jobStatus {
				dep.Status = jobStatus
			}
		}
	}
}

func (cp *connPackage) checkRunningJobsStatus() {
	var childJobs kbatch.JobList
	labelSelector := labels.SelectorFromSet(labels.Set{
		LabelWorkflowName: cp.mj.Name,
	})
	listOptions := &client.ListOptions{LabelSelector: labelSelector, Namespace: cp.mj.Namespace}
	err := cp.r.Client.List(cp.ctx, &childJobs, listOptions)
	if err != nil {
		cp.logger.Error(err, "Unable to list child jobs")
		return
	}

	activeJobCount := 0
	for _, childJob := range childJobs.Items {
		if childJob.Status.Active > 0 {
			activeJobCount++
		}
		for _, group := range cp.mj.Spec.Groups {
			for _, job := range group.Jobs {
				generatedJobName := jobNameGenerator(cp.mj.Name, group.Name, job.Name)
				if childJob.Name == generatedJobName {
					if childJob.Status.Succeeded > 0 && job.Status != ExecutionStatusSucceeded {
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Completed", "Job %s completed [prev: %s]", childJob.Name, job.Status)
						job.Status = ExecutionStatusSucceeded
						RecordJobSucceeded(cp.mj.Namespace, cp.mj.Name, group.Name)
					} else if childJob.Status.Failed > 0 && job.Status != ExecutionStatusFailed {
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failed", "Job %s failed [prev: %s]", childJob.Name, job.Status)
						job.Status = ExecutionStatusFailed
						RecordJobFailed(cp.mj.Namespace, cp.mj.Name, group.Name)
					} else if childJob.Status.Active > 0 && job.Status != ExecutionStatusRunning {
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Running", "Job %s running [prev: %s]", childJob.Name, job.Status)
						job.Status = ExecutionStatusRunning
					}
					cp.updateDependentJobs(generatedJobName, job.Status)
					continue
				}
			}
		}
	}
	SetActiveJobs(cp.mj.Namespace, cp.mj.Name, float64(activeJobCount))
}

func (cp *connPackage) runPendingJobs() {
	for _, group := range cp.mj.Spec.Groups {
		runGroup := false

		groupJobsCompleted := 0
		for _, job := range group.Jobs {
			if job.Status == ExecutionStatusSucceeded {
				groupJobsCompleted++
			}
		}
		if groupJobsCompleted == len(group.Jobs) {
			group.Status = ExecutionStatusSucceeded
			cp.updateDependentGroups(group.Name, group.Status)
			continue
		}

		approvedStatuses := []string{ExecutionStatusSucceeded, ExecutionStatusFailed, ExecutionStatusAborted}
		if pandati.ExistsInSlice(approvedStatuses, group.Status) {
			cp.updateDependentGroups(group.Name, group.Status)
		}

		approvedStatuses = []string{ExecutionStatusPending, ExecutionStatusRunning}
		if pandati.ExistsInSlice(approvedStatuses, group.Status) {
			if len(group.Dependencies) > 0 {
				groupsCompleted := 0
				for _, groupDep := range group.Dependencies {
					if groupDep.Status == ExecutionStatusSucceeded {
						groupsCompleted++
					}
					if groupDep.Status == ExecutionStatusFailed {
						group.Status = ExecutionStatusAborted
						cp.updateDependentGroups(group.Name, ExecutionStatusFailed)
					}
				}
				if groupsCompleted == len(group.Dependencies) {
					runGroup = true
				}
			} else {
				runGroup = true
			}

			if !runGroup {
				continue // not running the group as dependencies were not met
			} else {
				group.Status = ExecutionStatusRunning
				cp.updateDependentGroups(group.Name, ExecutionStatusRunning)

				for _, job := range group.Jobs {
					runJob := false
					if job.Status == ExecutionStatusPending {
						if len(job.Dependencies) > 0 {
							jobsCompleted := 0
							for _, jobDep := range job.Dependencies {
								if jobDep.Status == ExecutionStatusSucceeded {
									jobsCompleted++
								}
								if jobDep.Status == ExecutionStatusFailed {
									job.Status = ExecutionStatusAborted
									cp.updateDependentJobs(job.Name, ExecutionStatusFailed)
								}
							}
							if jobsCompleted == len(job.Dependencies) {
								runJob = true
							}
						} else {
							runJob = true
						}
					}

					if !runJob {
						continue // job is not ready as dependencies were not met
					} else {
						approvedStatuses = []string{ExecutionStatusRunning, ExecutionStatusFailed, ExecutionStatusAborted}
						if !pandati.ExistsInSlice(approvedStatuses, job.Status) {
							err := cp.executeJob(job, group)
							if err != nil {
								cp.logger.Error(err, "Unable to execute job", "job", job.Name, "group", group.Name)
								if !strings.Contains(err.Error(), "exists") {
									job.Status = ExecutionStatusFailed
									group.Status = ExecutionStatusFailed
									cp.updateDependentJobs(job.Name, ExecutionStatusFailed)
									cp.updateDependentGroups(group.Name, ExecutionStatusFailed)
									cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failed", "Job %s from group %s failed", job.Name, group.Name)
								}
								return
							}
							job.Status = ExecutionStatusRunning
							cp.updateDependentJobs(job.Name, ExecutionStatusRunning)
							cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Running", "Job %s from group %s running", job.Name, group.Name)
						}
					}
				}
			}
		}
	}
}

func (cp *connPackage) executeJob(j *jobsmanagerv1beta1.ManagedJobDefinition, g *jobsmanagerv1beta1.ManagedJobGroup) (err error) {
	generatedJobName := jobNameGenerator(cp.mj.Name, g.Name, j.Name)
	convertRetries := func(retries int) *int32 {
		if retries == 0 {
			return nil
		}
		// Ensure retries is within int32 bounds (max reasonable value for k8s backoff limit)
		if retries < 0 || retries > 100 {
			retries = 1 // default to 1 for invalid values
		}
		retries32 := int32(retries) // #nosec G115 - bounds checked above
		return &retries32
	}

	// compile labels
	labels := map[string]string{
		LabelWorkflowName: cp.mj.Name,
		LabelGroupName:    g.Name,
		LabelJobName:      generatedJobName,
		LabelJobID:        j.Name,
	}

	// merge labels with j.Parameters.Labels
	for k, v := range j.CompiledParams.Labels {
		labels[k] = v
	}

	annotations := map[string]string{}

	for k, v := range j.CompiledParams.Annotations {
		annotations[k] = v
	}

	k8sJob := kbatch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatedJobName,
			Namespace: cp.mj.Namespace,
		},
		Spec: kbatch.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        generatedJobName,
					Namespace:   cp.mj.Namespace,
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					Volumes:            j.CompiledParams.Volumes,
					ImagePullSecrets:   j.CompiledParams.ImagePullSecrets,
					ServiceAccountName: j.CompiledParams.ServiceAccount,
					Containers: []corev1.Container{
						{
							Name:            generatedJobName,
							Image:           j.Image,
							Args:            j.Args,
							ImagePullPolicy: getImagePullPolicy(j.CompiledParams.ImagePullPolicy),
							EnvFrom:         j.CompiledParams.FromEnv,
							Env:             j.CompiledParams.Env,
							VolumeMounts:    j.CompiledParams.VolumeMounts,
							Resources:       getResources(j.CompiledParams.Resources),
						},
					},
					RestartPolicy: corev1.RestartPolicy(j.CompiledParams.RestartPolicy),
				},
			},
			BackoffLimit: convertRetries(cp.mj.Spec.Retries),
		},
	}

	ownerRef, err := cp.getOwnerReference()
	if err != nil {
		return err
	}

	k8sJob.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	err = cp.r.Client.Create(cp.ctx, &k8sJob)
	if err != nil || pandati.IsZero(k8sJob) {
		return err
	}

	cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Created", "Created job %s", k8sJob.Name)
	RecordJobCreated(cp.mj.Namespace, cp.mj.Name, g.Name)
	return nil
}

func (cp *connPackage) checkOverallStatus() {
	groupsCompleted := 0
	negativeStatuses := []string{ExecutionStatusFailed, ExecutionStatusAborted}
	for _, group := range cp.mj.Spec.Groups {
		if group.Status == ExecutionStatusSucceeded {
			groupsCompleted++
		} else if pandati.ExistsInSlice(negativeStatuses, group.Status) {
			cp.mj.Status = ExecutionStatusFailed
			cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failure", "Run failed in group %s", group.Name)
		} else {
			continue
		}
	}

	if groupsCompleted == len(cp.mj.Spec.Groups) {
		if cp.mj.Status != ExecutionStatusSucceeded {
			cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Success", "Run completed successfuly")
		}
		cp.mj.Status = ExecutionStatusSucceeded
	} else {
		cp.mj.Status = ExecutionStatusRunning
	}
	if err := cp.r.Status().Update(cp.ctx, cp.mj); err != nil {
		cp.logger.Error(err, "Failed to update overall status")
	}
}
