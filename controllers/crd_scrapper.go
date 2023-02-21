package controllers

import (
	"fmt"
	"strings"

	"github.com/lukaszraczylo/pandati"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

/* Compile parameters from top to the job level */
type compiledParams struct {
	FromEnv          []corev1.EnvFromSource
	Env              []corev1.EnvVar
	Volumes          []corev1.Volume
	VolumeMounts     []corev1.VolumeMount
	ServiceAccount   string
	RestartPolicy    string
	ImagePullSecrets []corev1.LocalObjectReference
	ImagePullPolicy  string
	Labels           map[string]string
}

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
			if params.Labels != nil {
				for k, v := range params.Labels {
					cparams.Labels[k] = v
				}
			}
		}
	}
	return cparams
}

func (cp *connPackage) updateDependentJobs(completedJob string, jobStatus string) {
	for _, group := range cp.mj.Spec.Groups {
		for _, job := range group.Jobs {
			for _, dependency := range job.Dependencies {
				if dependency.Name == completedJob && dependency.Status != jobStatus {
					dependency.Status = jobStatus
				}
			}
		}
	}
}

func (cp *connPackage) updateDependentGroups(completedGroup string, jobStatus string) {
	for _, group := range cp.mj.Spec.Groups {
		for _, dependency := range group.Dependencies {
			if dependency.Name == completedGroup && dependency.Status != jobStatus {
				dependency.Status = jobStatus
			}
		}
	}
}

func (cp *connPackage) checkRunningJobsStatus() {
	var childJobs kbatch.JobList
	labelSelector := labels.SelectorFromSet(labels.Set{
		"jobmanager.raczylo.com/workflow-name": cp.mj.Name,
	})
	listOptions := &client.ListOptions{LabelSelector: labelSelector, Namespace: cp.mj.Namespace}
	err := cp.r.Client.List(cp.ctx, &childJobs, listOptions)
	if err != nil {
		log.Log.Info("Unable to list child jobs", "error", err.Error())
		return
	}

	for _, childJob := range childJobs.Items {
		for _, group := range cp.mj.Spec.Groups {
			for _, job := range group.Jobs {
				generatedJobName := jobNameGenerator(cp.mj.Name, group.Name, job.Name)
				if childJob.Name == generatedJobName {
					if childJob.Status.Succeeded > 0 && job.Status != ExecutionStatusSucceeded {
						job.Status = ExecutionStatusSucceeded
						cp.updateDependentJobs(generatedJobName, ExecutionStatusSucceeded)
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Completed", "Job %s completed", childJob.Name)
					} else if childJob.Status.Failed > 0 && job.Status != ExecutionStatusFailed {
						job.Status = ExecutionStatusFailed
						cp.updateDependentJobs(generatedJobName, ExecutionStatusFailed)
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failed", "Job %s failed", childJob.Name)
					} else if childJob.Status.Active > 0 && job.Status != ExecutionStatusRunning {
						job.Status = ExecutionStatusRunning
						cp.updateDependentJobs(generatedJobName, ExecutionStatusRunning)
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Running", "Job %s running", childJob.Name)
					}
				}
			}
		}
	}
}

func (cp *connPackage) runPendingJobs() {
	// originalMainJobDefinition := cp.mj.DeepCopy()
	for _, group := range cp.mj.Spec.Groups {
		run_group := false

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

		if group.Status == ExecutionStatusSucceeded || group.Status == ExecutionStatusFailed || group.Status == ExecutionStatusAborted {
			cp.updateDependentGroups(group.Name, group.Status)
		}

		if group.Status == ExecutionStatusPending || group.Status == ExecutionStatusRunning {
			if len(group.Dependencies) > 0 {
				groupsCompleted := 0
				for _, group_dependency := range group.Dependencies {
					if group_dependency.Status == ExecutionStatusSucceeded {
						groupsCompleted++
					}
					if group_dependency.Status == ExecutionStatusFailed {
						group.Status = ExecutionStatusAborted
						cp.updateDependentGroups(group.Name, ExecutionStatusFailed)
					}
				}
				if groupsCompleted == len(group.Dependencies) {
					run_group = true
				}
			} else {
				run_group = true
			}

			if !run_group {
				fmt.Println("Group "+group.Name+" is not running as dependencies were not met", group.Dependencies)
				continue // not running the group as dependencies were not met
			} else {
				group.Status = ExecutionStatusRunning
				cp.updateDependentGroups(group.Name, ExecutionStatusRunning)

				for _, job := range group.Jobs {
					run_job := false
					if job.Status == ExecutionStatusPending {
						if len(job.Dependencies) > 0 {
							jobsCompleted := 0
							for _, job_dependency := range job.Dependencies {
								if job_dependency.Status == ExecutionStatusSucceeded {
									jobsCompleted++
								}
								if job_dependency.Status == ExecutionStatusFailed {
									job.Status = ExecutionStatusAborted
									cp.updateDependentJobs(job.Name, ExecutionStatusFailed)
								}
							}
							if jobsCompleted == len(job.Dependencies) {
								run_job = true
							}
						} else {
							run_job = true
						}
					}

					if !run_job {
						continue // job is not ready as dependencies were not met
					} else {
						if job.Status != ExecutionStatusRunning && job.Status != ExecutionStatusFailed && job.Status != ExecutionStatusSucceeded {
							job.Status = ExecutionStatusRunning
							cp.updateDependentJobs(job.Name, ExecutionStatusRunning)
							cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Running", "Job %s from group %s running", job.Name, group.Name)
							err := cp.executeJob(job, group)
							if err != nil {
								log.Log.Info("Unable to execute job", "error", err.Error())
								if !strings.Contains(err.Error(), "already exists") {
									job.Status = ExecutionStatusFailed
									group.Status = ExecutionStatusFailed
									cp.updateDependentJobs(job.Name, ExecutionStatusFailed)
									cp.updateDependentGroups(group.Name, ExecutionStatusFailed)
									cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failed", "Job %s from group %s failed", job.Name, group.Name)
								}
							}
						}
					}
				}
			}

			fmt.Println("Running group: ", group.Name, " with status: ", group.Status, " accepted: ", run_group)
		}
	}
}

func (cp *connPackage) executeJob(j *jobsmanagerv1beta1.ManagedJobDefinition, g *jobsmanagerv1beta1.ManagedJobGroup) (err error) {
	generatedJobName := jobNameGenerator(cp.mj.Name, g.Name, j.Name)
	convertRetries := func(retries int) *int32 {
		if retries == 0 {
			return nil
		}
		retries32 := int32(retries)
		return &retries32
	}

	// compile labels
	labels := map[string]string{
		"jobmanager.raczylo.com/workflow-name": cp.mj.Name,
		"jobmanager.raczylo.com/group-name":    g.Name,
		"jobmanager.raczylo.com/job-name":      generatedJobName,
		"jobmanager.raczylo.com/job-id":        j.Name,
	}

	// merge labels with j.Parameters.Labels
	for k, v := range j.CompiledParams.Labels {
		labels[k] = v
	}

	job_handler := kbatch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      generatedJobName,
			Namespace: cp.mj.Namespace,
		},
		Spec: kbatch.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generatedJobName,
					Namespace: cp.mj.Namespace,
					Labels:    labels,
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
							ImagePullPolicy: corev1.PullPolicy(j.CompiledParams.ImagePullPolicy),
							EnvFrom:         j.CompiledParams.FromEnv,
							Env:             j.CompiledParams.Env,
							VolumeMounts:    j.CompiledParams.VolumeMounts,
						},
					},
					RestartPolicy: corev1.RestartPolicy(j.CompiledParams.RestartPolicy),
				},
			},
			BackoffLimit: convertRetries(cp.mj.Spec.Retries),
		},
	}

	getMetaRefForWorkflowData, err := cp.getOwnerReference()
	if err != nil {
		return err
	}

	job_handler.SetOwnerReferences([]metav1.OwnerReference{getMetaRefForWorkflowData})

	err = cp.r.Client.Create(cp.ctx, &job_handler)
	if err != nil || pandati.IsZero(job_handler) {
		log.Log.Info("Unable to create job", "job", job_handler.Name, "error", err.Error())
		return err
	}

	cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Created", "Created job %s", job_handler.Name)
	return nil
}

func (cp *connPackage) checkOverallStatus() {
	groupsCompleted := 0
	for _, group := range cp.mj.Spec.Groups {
		if group.Status == ExecutionStatusSucceeded {
			groupsCompleted++
		} else if group.Status == ExecutionStatusFailed || group.Status == ExecutionStatusAborted {
			cp.mj.Status = ExecutionStatusFailed
			cp.mj.Spec.Status = cp.mj.Status
			cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeWarning, "Failure", "Run failed")
		}
	}

	if groupsCompleted == len(cp.mj.Spec.Groups) {
		cp.mj.Status = ExecutionStatusSucceeded
		cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Success", "Run completed successfuly")
	} else {
		cp.mj.Status = ExecutionStatusRunning
	}
	cp.mj.Spec.Status = cp.mj.Status
	cp.r.Status().Update(cp.ctx, cp.mj)
}
