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

	"github.com/lukaszraczylo/pandati"
	kbatch "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
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

func (r *ManagedJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	cp := &connPackage{
		r:              r,
		ctx:            ctx,
		req:            req,
		dependencyTree: nil,
	}

	var managedJob jobsmanagerv1beta1.ManagedJob
	if err := r.Get(ctx, req.NamespacedName, &managedJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cp.mj = &managedJob

	originalMainJobDefinition := cp.mj.DeepCopy()
	cp.generateDependencyTree()

	// TODO: Re-enable after testing
	cp.checkRunningJobsStatus()
	cp.runPendingJobs()
	cp.checkOverallStatus()

	_, theSame, _ := pandati.CompareStructsReplaced(originalMainJobDefinition, cp.mj)
	if !theSame {
		cp.updateCRDStatusDirectly()
	}
	// fmt.Printf("Reconcile: %# v", pretty.Formatter(r.Updater))

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jobsmanagerv1beta1.ManagedJob{}).
		Owns(&kbatch.Job{}).
		Complete(r)
}
