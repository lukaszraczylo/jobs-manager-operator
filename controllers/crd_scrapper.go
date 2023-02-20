package controllers

import (
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

type previousJobsAndGroups struct {
	GroupID string
	JobID   string
}

func (cp *connPackage) buildJobsDependencyTree() error {
	changePending := false
	for i, group := range cp.mj.Spec.Groups {
		var group_previous string
		if group.Parallel {
			group_previous = group.Name
		} else {
			if i > 0 {
				group_previous = cp.mj.Spec.Groups[i-1].Name
			} else {
				group_previous = group.Name
			}
		}
		for j, job := range group.Jobs {
			if job.Dependencies.Group == "" || job.Dependencies.Job == "" {
				changePending = true
			}
			job.Dependencies.Group = group_previous
			if job.Parallel {
				job.Dependencies.Job = job.Name
			} else {
				if j > 0 {
					job.Dependencies.Job = group.Jobs[j-1].Name
				} else {
					job.Dependencies.Job = job.Name
				}
			}
		}
	}
	if changePending {
		cp.updateCRDStatusDirectly()
	}
	return nil
}

func (cp *connPackage) checkJobStatus() {
	var childJobs kbatch.JobList
	labelSelector := labels.SelectorFromSet(labels.Set{
		"jobmanager.raczylo.com/workflow-name": cp.mj.Name,
	})
	listOptions := &client.ListOptions{LabelSelector: labelSelector, Namespace: cp.mj.Namespace}

	err := cp.r.Client.List(cp.ctx, &childJobs, listOptions)
	if err != nil {
		// log.Log.Error(err, "unable to list child jobs")
		return
	}
	changePresent := false
	for _, childJob := range childJobs.Items {
		for _, group := range cp.mj.Spec.Groups {
			for _, job := range group.Jobs {
				generatedJobName := jobNameGenerator(cp.mj.Name, group.Name, job.Name)
				if childJob.Name == generatedJobName {
					if childJob.Status.Succeeded > 0 && job.Status != ExecutionStatusSucceeded {
						job.Status = ExecutionStatusSucceeded
						changePresent = true
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Completed", "Job %s completed", childJob.Name)
					} else if childJob.Status.Failed > 0 && job.Status != ExecutionStatusFailed {
						job.Status = ExecutionStatusFailed
						changePresent = true
						cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Failed", "Job %s failed", childJob.Name)
					} else if childJob.Status.Active > 0 && job.Status != ExecutionStatusRunning {
						changePresent = true
						job.Status = ExecutionStatusRunning
					}
				}
			}

		}
	}
	if changePresent {
		cp.updateCRDStatusDirectly()
	}
}

func (cp *connPackage) runPendingJobs() {
	copyMJ := cp.mj.DeepCopy()
	changePresent := false
	for _, group := range cp.mj.Spec.Groups {
		for _, job := range group.Jobs {
			if job.Status == ExecutionStatusPending {
				if job.Dependencies.Group == group.Name && job.Dependencies.Job == job.Name {
					job.CompiledParams = cp.compileParameters(job.Params, group.Params, cp.mj.Spec.Params)
					err := cp.executeJob(job, group)
					if err != nil {
						// log.Log.Info("Unable to execute job", "group", group.Name, "job", job.Name, "error", err)
						continue
					}
					job.Status = ExecutionStatusRunning
					changePresent = true
					continue
				} else {
					for _, group2 := range copyMJ.Spec.Groups {
						for _, job2 := range group2.Jobs {
							if job2.Name == job.Dependencies.Job && group2.Name == job.Dependencies.Group {
								switch job2.Status {
								case ExecutionStatusSucceeded:
									job.CompiledParams = cp.compileParameters(job.Params, group.Params, cp.mj.Spec.Params)
									err := cp.executeJob(job, group)
									if err != nil {
										log.Log.Info("Unable to execute job", "group", group.Name, "job", job.Name, "error", err)
										continue
									}
									job.Status = ExecutionStatusRunning
									changePresent = true
								case ExecutionStatusFailed:
									job.Status = ExecutionStatusAborted
									changePresent = true
								}
							}
						}
					}
				}
			}
		}
	}
	if changePresent {
		cp.updateCRDStatusDirectly()
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
		log.Log.Error(err, "Unable to create job", "job", job_handler.Name)
		return err
	}

	cp.r.Recorder.Eventf(cp.mj, corev1.EventTypeNormal, "Created", "Created job %s", job_handler.Name)
	return nil
}
