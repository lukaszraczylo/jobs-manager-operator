package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kr/pretty"
	"github.com/lukaszraczylo/pandati"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"raczylo.com/jobs-manager-operator/api/v1beta1"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	r   *ManagedJobReconciler
	ctx context.Context
	req ctrl.Request
	mtx sync.Mutex
	mj  *jobsmanagerv1beta1.ManagedJob
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

type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

func (cp *connPackage) getLatestMainJobAndPatch(proposedChanges *jobsmanagerv1beta1.ManagedJob) (bool, error) {
	mj := jobsmanagerv1beta1.ManagedJob{}
	err := cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, &mj)
	if err != nil {
		return false, err
	}
	diff, diffIdentical, _ := pandati.CompareStructsReplaced(mj, proposedChanges)
	if !diffIdentical {
		if err != nil {
			log.Log.Error(err, "Unable to marshal ManagedJob")
		}
		var payloadBytesArr []patchStringValue
		for _, d := range diff {
			if strings.HasPrefix(d.Key, "/spec") {
				operation := "replace"
				if d.OldValue == "" || d.OldValue == nil {
					operation = "add"
				}

				payload := patchStringValue{
					Op:    operation,
					Path:  d.Key,
					Value: d.Value.(string),
				}
				payloadBytesArr = append(payloadBytesArr, payload)
			}
		}
		patchPayload, err := json.Marshal(payloadBytesArr)
		if err != nil {
			log.Log.Error(err, "Unable to marshal ManagedJob")
		}
		fmt.Printf("PatchPayload: %# v", pretty.Formatter(fmt.Sprintf("%s", patchPayload)))
		kubepath := client.RawPatch(types.JSONPatchType, patchPayload)
		err = cp.r.Client.Patch(cp.ctx, &mj, kubepath)
		// if err != nil {
		// 	log.Log.Error(err, "Unable to patch ManagedJob")
		// }
		return true, err
	}
	return false, nil
}

func (cp *connPackage) updateCRDStatusDirectly() error {
	cp.mtx.Lock()
	// defer cp.mtx.Unlock()

	// val, err := cp.getLatestMainJobAndPatch(cp.mj)
	// if err != nil {
	// 	log.Log.Error(err, "Unable to get latest ManagedJob")
	// 	return err
	// }

	// if !val && err == nil {
	err := cp.r.Update(cp.ctx, cp.mj)
	if err != nil {
		// log.Log.Info("Error", err.Error(), "more", "Unable to update ManagedJob status directly")
	}
	// get updated ManagedJob
	err = cp.r.Client.Get(cp.ctx, cp.req.NamespacedName, cp.mj)
	if err != nil {
		log.Log.Error(err, "Unable to get updated ManagedJob")
	}
	// }
	cp.mtx.Unlock()
	return err
}
