package controllers

import (
	"context"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"raczylo.com/jobs-manager-operator/api/v1beta1"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func jobNameGenerator(name ...string) string {
	// join name parts with "-" and convert to lowercase
	return strings.ToLower(strings.Join(name, "-"))
}

type jobStatusUpdate struct {
	Job             *jobsmanagerv1beta1.ManagedJob
	PatchedResource string
	Status          string
}

type connPackage struct {
	r              *ManagedJobReconciler
	ctx            context.Context
	req            ctrl.Request
	mtx            sync.Mutex
	mj             *jobsmanagerv1beta1.ManagedJob
	dependencyTree Tree
}

func (cp *connPackage) getOwnerReference() (metav1.OwnerReference, error) {
	mj := &jobsmanagerv1beta1.ManagedJob{}
	err := cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, mj)
	if err != nil {
		return metav1.OwnerReference{}, err
	}
	t := true
	return metav1.OwnerReference{
		APIVersion: v1beta1.GroupVersion.String(),
		Kind:       "ManagedJob",
		Name:       mj.Name,
		UID:        mj.UID,
		Controller: &t,
	}, nil
}

func (cp *connPackage) updateCRDStatusDirectly() error {
	cp.mtx.Lock()
	err := cp.r.Update(cp.ctx, cp.mj)
	if err != nil {
		// log.Log.Info("Error", err.Error(), "more", "Unable to update ManagedJob status directly")
	}
	// get updated ManagedJob
	err = cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, cp.mj)
	if err != nil {
		log.Log.Error(err, "Unable to get updated ManagedJob")
	}
	cp.mtx.Unlock()
	return err
}
