package controllers

import (
	"context"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func jobNameGenerator(name ...string) string {
	// join name parts with "-" and convert to lowercase
	return strings.ToLower(strings.Join(name, "-"))
}

type connPackage struct {
	r      *ManagedJobReconciler
	ctx    context.Context
	req    ctrl.Request
	mtx    sync.Mutex
	mj     *jobsmanagerv1beta1.ManagedJob
	logger logr.Logger
	// jobDepMap maps job names to dependencies that reference them (for O(1) lookup)
	jobDepMap map[string][]*jobsmanagerv1beta1.ManagedJobDependencies
	// groupDepMap maps group names to dependencies that reference them (for O(1) lookup)
	groupDepMap map[string][]*jobsmanagerv1beta1.ManagedJobDependencies
}

// buildDependencyMaps constructs lookup maps for efficient dependency status updates.
// This converts O(n*m) lookups to O(1) by mapping job/group names to their dependents.
func (cp *connPackage) buildDependencyMaps() {
	cp.jobDepMap = make(map[string][]*jobsmanagerv1beta1.ManagedJobDependencies)
	cp.groupDepMap = make(map[string][]*jobsmanagerv1beta1.ManagedJobDependencies)

	for _, group := range cp.mj.Spec.Groups {
		// Map group dependencies
		for _, dep := range group.Dependencies {
			cp.groupDepMap[dep.Name] = append(cp.groupDepMap[dep.Name], dep)
		}
		// Map job dependencies
		for _, job := range group.Jobs {
			for _, dep := range job.Dependencies {
				cp.jobDepMap[dep.Name] = append(cp.jobDepMap[dep.Name], dep)
			}
		}
	}
}

func (cp *connPackage) getOwnerReference() (metav1.OwnerReference, error) {
	mj := &jobsmanagerv1beta1.ManagedJob{}
	err := cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, mj)
	if err != nil {
		return metav1.OwnerReference{}, err
	}
	t := true
	return metav1.OwnerReference{
		APIVersion: jobsmanagerv1beta1.GroupVersion.String(),
		Kind:       "ManagedJob",
		Name:       mj.Name,
		UID:        mj.UID,
		Controller: &t,
	}, nil
}

func (cp *connPackage) updateCRDStatusDirectly() error {
	cp.mtx.Lock()
	defer cp.mtx.Unlock()

	if err := cp.r.Update(cp.ctx, cp.mj); err != nil {
		cp.logger.Error(err, "Unable to update ManagedJob status directly")
		return err
	}

	if err := cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, cp.mj); err != nil {
		cp.logger.Error(err, "Unable to get updated ManagedJob")
		return err
	}

	return nil
}

// getImagePullPolicy returns the specified pull policy or IfNotPresent as default
func getImagePullPolicy(policy string) corev1.PullPolicy {
	if policy == "" {
		return corev1.PullIfNotPresent
	}
	return corev1.PullPolicy(policy)
}

// getResources returns the resource requirements or empty requirements if nil
func getResources(resources *corev1.ResourceRequirements) corev1.ResourceRequirements {
	if resources == nil {
		return corev1.ResourceRequirements{}
	}
	return *resources
}
