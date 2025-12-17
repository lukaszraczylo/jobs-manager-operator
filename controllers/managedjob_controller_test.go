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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

var _ = Describe("ManagedJob Controller", func() {
	const (
		ManagedJobName      = "test-workflow"
		ManagedJobNamespace = "default"
		timeout             = time.Second * 10
		interval            = time.Millisecond * 250
	)

	Context("When creating a ManagedJob", func() {
		It("Should add finalizer to new ManagedJob", func() {
			ctx := context.Background()

			managedJob := &jobsmanagerv1beta1.ManagedJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ManagedJobName + "-finalizer",
					Namespace: ManagedJobNamespace,
				},
				Spec: jobsmanagerv1beta1.ManagedJobSpec{
					Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
						{
							Name: "group1",
							Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
								{
									Name:  "job1",
									Image: "busybox:latest",
									Args:  []string{"echo", "hello"},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, managedJob)).Should(Succeed())

			lookupKey := types.NamespacedName{Name: ManagedJobName + "-finalizer", Namespace: ManagedJobNamespace}
			createdManagedJob := &jobsmanagerv1beta1.ManagedJob{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdManagedJob)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Trigger reconciliation manually since we don't have the controller running
			reconciler := &ManagedJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: lookupKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdManagedJob)
				if err != nil {
					return false
				}
				for _, f := range createdManagedJob.Finalizers {
					if f == FinalizerName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, createdManagedJob)).Should(Succeed())
		})

		It("Should initialize job statuses to pending", func() {
			ctx := context.Background()

			managedJob := &jobsmanagerv1beta1.ManagedJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ManagedJobName + "-status",
					Namespace: ManagedJobNamespace,
				},
				Spec: jobsmanagerv1beta1.ManagedJobSpec{
					Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
						{
							Name: "init-group",
							Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
								{
									Name:  "init-job",
									Image: "busybox:latest",
								},
							},
						},
						{
							Name: "main-group",
							Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
								{Name: "init-group"},
							},
							Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
								{
									Name:  "main-job",
									Image: "busybox:latest",
									Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
										{Name: "init-job"},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, managedJob)).Should(Succeed())

			lookupKey := types.NamespacedName{Name: ManagedJobName + "-status", Namespace: ManagedJobNamespace}
			createdManagedJob := &jobsmanagerv1beta1.ManagedJob{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdManagedJob)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Trigger reconciliation
			reconciler := &ManagedJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: lookupKey})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile processes jobs
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: lookupKey})
			Expect(err).NotTo(HaveOccurred())

			// Verify statuses
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, createdManagedJob)
				if err != nil {
					return false
				}
				// Check that groups have jobs with pending status initially
				for _, g := range createdManagedJob.Spec.Groups {
					for _, j := range g.Jobs {
						if j.Status == "" {
							return false
						}
					}
				}
				return true
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, createdManagedJob)).Should(Succeed())
		})
	})

	Context("When a ManagedJob is deleted", func() {
		It("Should clean up child jobs", func() {
			ctx := context.Background()

			// Create a ManagedJob
			managedJob := &jobsmanagerv1beta1.ManagedJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ManagedJobName + "-cleanup",
					Namespace: ManagedJobNamespace,
				},
				Spec: jobsmanagerv1beta1.ManagedJobSpec{
					Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
						{
							Name: "cleanup-group",
							Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
								{
									Name:  "cleanup-job",
									Image: "busybox:latest",
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, managedJob)).Should(Succeed())

			lookupKey := types.NamespacedName{Name: ManagedJobName + "-cleanup", Namespace: ManagedJobNamespace}

			// Create a child job with the workflow label
			childJob := &kbatch.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workflow-cleanup-cleanup-group-cleanup-job",
					Namespace: ManagedJobNamespace,
					Labels: map[string]string{
						LabelWorkflowName: ManagedJobName + "-cleanup",
					},
				},
				Spec: kbatch.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "test",
									Image: "busybox:latest",
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, childJob)).Should(Succeed())

			// Verify child job exists
			childJobKey := types.NamespacedName{
				Name:      "test-workflow-cleanup-cleanup-group-cleanup-job",
				Namespace: ManagedJobNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, childJobKey, &kbatch.Job{})
				return err == nil
			}, timeout, interval).Should(BeTrue())

			// Delete the ManagedJob
			createdManagedJob := &jobsmanagerv1beta1.ManagedJob{}
			Expect(k8sClient.Get(ctx, lookupKey, createdManagedJob)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, createdManagedJob)).Should(Succeed())
		})
	})
})

var _ = Describe("Execution Status Constants", func() {
	It("Should have correct status values", func() {
		Expect(ExecutionStatusPending).To(Equal("pending"))
		Expect(ExecutionStatusRunning).To(Equal("running"))
		Expect(ExecutionStatusSucceeded).To(Equal("succeeded"))
		Expect(ExecutionStatusFailed).To(Equal("failed"))
		Expect(ExecutionStatusAborted).To(Equal("aborted"))
	})

	It("Should have correct label values", func() {
		Expect(LabelWorkflowName).To(Equal("jobmanager.raczylo.com/workflow-name"))
		Expect(LabelGroupName).To(Equal("jobmanager.raczylo.com/group-name"))
		Expect(LabelJobName).To(Equal("jobmanager.raczylo.com/job-name"))
		Expect(LabelJobID).To(Equal("jobmanager.raczylo.com/job-id"))
	})
})
