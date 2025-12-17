/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"time"

	"github.com/lukaszraczylo/pandati"
	kbatch "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

const (
	// RequeueDelay is the time to wait before requeuing when jobs are running
	RequeueDelay = 30 * time.Second
)

// ManagedJobReconciler reconciles a ManagedJob object
type ManagedJobReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=jobsmanager.raczylo.com,resources=managedjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=jobsmanager.raczylo.com,resources=managedjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=jobsmanager.raczylo.com,resources=managedjobs/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=events,verbs=create;update;patch;delete;get;list;watch

// Reconcile ensures ManagedJob workflows progress toward completion.
// It orchestrates job execution respecting dependencies, manages retries,
// and tracks overall workflow status.
func (r *ManagedJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("managedJob", req.NamespacedName)

	var managedJob jobsmanagerv1beta1.ManagedJob
	if err := r.Get(ctx, req.NamespacedName, &managedJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cp := &connPackage{
		r:      r,
		ctx:    ctx,
		req:    req,
		logger: logger,
		mj:     &managedJob,
	}

	// Handle deletion with finalizer
	if !managedJob.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, cp)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&managedJob, FinalizerName) {
		controllerutil.AddFinalizer(&managedJob, FinalizerName)
		if err := r.Update(ctx, &managedJob); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	originalMainJobDefinition := cp.mj.DeepCopy()
	cp.generateDependencyTree()
	cp.buildDependencyMaps() // Build lookup maps for O(1) dependency updates
	_, theSame, _ := pandati.CompareStructsReplaced(originalMainJobDefinition, cp.mj)
	if !theSame {
		if err := cp.updateCRDStatusDirectly(); err != nil {
			logger.Error(err, "Failed to update CRD status after dependency tree generation")
		}
		return ctrl.Result{}, nil
	}
	originalMainJobDefinition = cp.mj.DeepCopy()

	cp.checkRunningJobsStatus()
	cp.runPendingJobs()

	_, theSame, _ = pandati.CompareStructsReplaced(originalMainJobDefinition, cp.mj)
	if !theSame {
		if err := cp.updateCRDStatusDirectly(); err != nil {
			logger.Error(err, "Failed to update CRD status after job processing")
		}
	}

	cp.checkOverallStatus()

	// If workflow is still running, requeue after a delay to check status
	if cp.mj.Status == ExecutionStatusRunning {
		return ctrl.Result{RequeueAfter: RequeueDelay}, nil
	}

	return ctrl.Result{}, nil
}

// handleDeletion cleans up child jobs before removing the finalizer
func (r *ManagedJobReconciler) handleDeletion(ctx context.Context, cp *connPackage) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(cp.mj, FinalizerName) {
		return ctrl.Result{}, nil
	}

	cp.logger.Info("Cleaning up child jobs before deletion")

	// Delete all child jobs
	if err := r.deleteChildJobs(ctx, cp); err != nil {
		cp.logger.Error(err, "Failed to delete child jobs")
		return ctrl.Result{}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(cp.mj, FinalizerName)
	if err := r.Update(ctx, cp.mj); err != nil {
		cp.logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	cp.logger.Info("Successfully cleaned up ManagedJob")
	return ctrl.Result{}, nil
}

// deleteChildJobs removes all jobs owned by this ManagedJob
func (r *ManagedJobReconciler) deleteChildJobs(ctx context.Context, cp *connPackage) error {
	var childJobs kbatch.JobList
	labelSelector := labels.SelectorFromSet(labels.Set{
		LabelWorkflowName: cp.mj.Name,
	})
	listOptions := &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     cp.mj.Namespace,
	}

	if err := r.Client.List(ctx, &childJobs, listOptions); err != nil {
		return err
	}

	for i := range childJobs.Items {
		job := &childJobs.Items[i]
		if err := r.Client.Delete(ctx, job, client.PropagationPolicy("Background")); err != nil {
			cp.logger.Error(err, "Failed to delete child job", "job", job.Name)
			// Continue trying to delete other jobs
		} else {
			cp.logger.Info("Deleted child job", "job", job.Name)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jobsmanagerv1beta1.ManagedJob{}).
		Owns(&kbatch.Job{}).
		Complete(r)
}
