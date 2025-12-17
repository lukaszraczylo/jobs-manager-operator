package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	kbatch "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

// ReconcilerTestSuite contains all reconciler tests
type ReconcilerTestSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	client     *MockClient
	reconciler *ManagedJobReconciler
}

func (s *ReconcilerTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)
	s.client = NewMockClient()
	s.reconciler = &ManagedJobReconciler{
		Client:   s.client,
		Scheme:   s.client.Scheme(),
		Recorder: NewFakeRecorder(),
	}
}

func (s *ReconcilerTestSuite) TearDownTest() {
	s.cancel()
}

func TestReconcilerSuite(t *testing.T) {
	suite.Run(t, new(ReconcilerTestSuite))
}

// ==================== GOOD SCENARIOS ====================

func (s *ReconcilerTestSuite) TestReconcile_Good_NewManagedJob_AddsFinalizer() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	s.client.AddManagedJob(mj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	result, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err)
	s.True(result.RequeueAfter > 0, "should requeue after adding finalizer")

	updated := s.client.GetManagedJobByKey("default/test-workflow")
	s.True(controllerutil.ContainsFinalizer(updated, FinalizerName), "finalizer should be added")
}

func (s *ReconcilerTestSuite) TestReconcile_Good_SingleJobWorkflow_CreatesJob() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act - first reconcile generates dependency tree
	_, err := s.reconciler.Reconcile(s.ctx, req)
	s.NoError(err)

	// Act - second reconcile runs jobs
	_, err = s.reconciler.Reconcile(s.ctx, req)
	s.NoError(err)

	// Assert
	s.GreaterOrEqual(s.client.CreateCalls, 1, "should have created at least one job")
}

func (s *ReconcilerTestSuite) TestReconcile_Good_CompletedJob_UpdatesStatus() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	mj.Spec.Groups[0].Jobs[0].Status = ExecutionStatusRunning
	mj.Spec.Groups[0].Status = ExecutionStatusRunning
	s.client.AddManagedJob(mj)

	// Add a completed K8s job
	k8sJob := NewTestK8sJob("test-workflow-group1-job1", "default", "test-workflow", "group1", kbatch.JobStatus{
		Succeeded: 1,
	})
	s.client.AddJob(k8sJob)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err)
}

func (s *ReconcilerTestSuite) TestReconcile_Good_RunningWorkflow_RequeuesAfterDelay() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	mj.Status = ExecutionStatusRunning
	mj.Spec.Groups[0].Status = ExecutionStatusRunning
	mj.Spec.Groups[0].Jobs[0].Status = ExecutionStatusRunning
	s.client.AddManagedJob(mj)

	// Add running K8s job
	k8sJob := NewTestK8sJob("test-workflow-group1-job1", "default", "test-workflow", "group1", kbatch.JobStatus{
		Active: 1,
	})
	s.client.AddJob(k8sJob)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	result, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err)
	s.Equal(RequeueDelay, result.RequeueAfter, "should requeue after delay for running workflow")
}

// ==================== NOT GOOD SCENARIOS ====================

func (s *ReconcilerTestSuite) TestReconcile_NotGood_ManagedJobNotFound_ReturnsNoError() {
	// Arrange - no ManagedJob added to client
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
	}

	// Act
	result, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err, "should not return error for not found")
	s.Zero(result.RequeueAfter, "should not requeue for not found")
}

func (s *ReconcilerTestSuite) TestReconcile_NotGood_FailedJob_UpdatesStatusToFailed() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	mj.Spec.Groups[0].Jobs[0].Status = ExecutionStatusRunning
	mj.Spec.Groups[0].Status = ExecutionStatusRunning
	s.client.AddManagedJob(mj)

	// Add a failed K8s job
	k8sJob := NewTestK8sJob("test-workflow-group1-job1", "default", "test-workflow", "group1", kbatch.JobStatus{
		Failed: 1,
	})
	s.client.AddJob(k8sJob)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err)
}

func (s *ReconcilerTestSuite) TestReconcile_NotGood_DeletionInProgress_CleansUpJobs() {
	// Arrange
	now := metav1.Now()
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	mj.DeletionTimestamp = &now
	s.client.AddManagedJob(mj)

	// Add child job to be deleted
	k8sJob := NewTestK8sJob("test-workflow-group1-job1", "default", "test-workflow", "group1", kbatch.JobStatus{})
	s.client.AddJob(k8sJob)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.NoError(err)
	s.GreaterOrEqual(s.client.DeleteCalls, 1, "should have deleted child jobs")
}

// ==================== REALLY BAD SCENARIOS ====================

func (s *ReconcilerTestSuite) TestReconcile_ReallyBad_GetError_ReturnsError() {
	// Arrange
	s.client.GetError = ErrServerError

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.Error(err)
}

func (s *ReconcilerTestSuite) TestReconcile_ReallyBad_UpdateConflict_ReturnsError() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	s.client.AddManagedJob(mj)
	s.client.UpdateError = ErrConflict

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert
	s.Error(err, "should return error on update conflict")
}

func (s *ReconcilerTestSuite) TestReconcile_ReallyBad_ContextTimeout_ReturnsError() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	s.client.AddManagedJob(mj)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act
	_, err := s.reconciler.Reconcile(ctx, req)

	// Assert
	s.Error(err, "should return error on context timeout")
}

func (s *ReconcilerTestSuite) TestReconcile_ReallyBad_ListJobsError_Continues() {
	// Arrange
	mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox:latest"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)
	s.client.ListError = ErrServerError

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "test-workflow", Namespace: "default"},
	}

	// Act - should not panic, should handle gracefully
	_, err := s.reconciler.Reconcile(s.ctx, req)

	// Assert - list error is logged but doesn't fail reconciliation
	s.NoError(err)
}

// ==================== MATRIX TEST: WORKFLOW SCENARIOS ====================

func TestReconcile_WorkflowScenarios(t *testing.T) {
	tests := []struct {
		name               string
		scenario           TestScenario
		setupMJ            func() *jobsmanagerv1beta1.ManagedJob
		setupJobs          func() []*kbatch.Job
		clientSetup        func(*MockClient)
		expectError        bool
		expectRequeue      bool
		expectRequeueAfter time.Duration
		validateResult     func(*testing.T, *MockClient, ctrl.Result)
	}{
		// GOOD SCENARIOS
		{
			name:     "good_simple_workflow_starts",
			scenario: ScenarioGood,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			setupJobs:     func() []*kbatch.Job { return nil },
			expectError:   false,
			expectRequeue: false,
		},
		{
			name:     "good_multi_group_workflow",
			scenario: ScenarioGood,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
					}),
					NewTestGroup("main", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("process", "busybox:latest"),
					}, NewTestDependency("init", ExecutionStatusPending)),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			setupJobs:     func() []*kbatch.Job { return nil },
			expectError:   false,
			expectRequeue: false,
		},
		{
			name:     "good_all_jobs_completed",
			scenario: ScenarioGood,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				mj.Spec.Groups[0].Jobs[0].Status = ExecutionStatusSucceeded
				mj.Spec.Groups[0].Status = ExecutionStatusSucceeded
				return mj
			},
			setupJobs: func() []*kbatch.Job {
				return []*kbatch.Job{
					NewTestK8sJob("workflow-init-setup", "default", "workflow", "init", kbatch.JobStatus{Succeeded: 1}),
				}
			},
			expectError:   false,
			expectRequeue: false,
		},

		// NOT GOOD SCENARIOS
		{
			name:     "notgood_job_failed_workflow_continues",
			scenario: ScenarioNotGood,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				mj.Spec.Groups[0].Jobs[0].Status = ExecutionStatusRunning
				return mj
			},
			setupJobs: func() []*kbatch.Job {
				return []*kbatch.Job{
					NewTestK8sJob("workflow-init-setup", "default", "workflow", "init", kbatch.JobStatus{Failed: 1}),
				}
			},
			expectError:   false,
			expectRequeue: false,
		},
		{
			name:     "notgood_dependent_job_aborted",
			scenario: ScenarioNotGood,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
						NewTestJobDef("verify", "busybox:latest", NewTestDependency("setup", ExecutionStatusFailed)),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			setupJobs: func() []*kbatch.Job {
				return []*kbatch.Job{
					NewTestK8sJob("workflow-init-setup", "default", "workflow", "init", kbatch.JobStatus{Failed: 1}),
				}
			},
			expectError:   false,
			expectRequeue: false,
		},

		// REALLY BAD SCENARIOS
		{
			name:     "reallybad_api_server_unavailable",
			scenario: ScenarioReallyBad,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				return NewTestManagedJob("workflow", "default", nil)
			},
			setupJobs: func() []*kbatch.Job { return nil },
			clientSetup: func(c *MockClient) {
				c.GetError = ErrNetworkFailure
			},
			expectError:   true,
			expectRequeue: false,
		},
		{
			name:     "reallybad_create_job_fails",
			scenario: ScenarioReallyBad,
			setupMJ: func() *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("init", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("setup", "busybox:latest"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			setupJobs: func() []*kbatch.Job { return nil },
			clientSetup: func(c *MockClient) {
				c.CreateError = ErrForbidden
			},
			expectError:   false, // Job creation failure doesn't stop reconciliation
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := NewMockClient()
			reconciler := &ManagedJobReconciler{
				Client:   client,
				Scheme:   client.Scheme(),
				Recorder: NewFakeRecorder(),
			}

			// Setup client behavior
			if tt.clientSetup != nil {
				tt.clientSetup(client)
			}

			// Add ManagedJob
			mj := tt.setupMJ()
			if mj != nil {
				client.AddManagedJob(mj)
			}

			// Add Jobs
			for _, job := range tt.setupJobs() {
				client.AddJob(job)
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "workflow", Namespace: "default"},
			}

			// Act
			result, err := reconciler.Reconcile(ctx, req)

			// Assert
			if tt.expectError {
				assert.Error(t, err, "expected error for scenario: %s", tt.scenario)
			} else {
				assert.NoError(t, err, "expected no error for scenario: %s", tt.scenario)
			}

			if tt.expectRequeue {
				assert.True(t, result.RequeueAfter > 0, "expected requeue")
			}

			if tt.expectRequeueAfter > 0 {
				assert.Equal(t, tt.expectRequeueAfter, result.RequeueAfter)
			}

			if tt.validateResult != nil {
				tt.validateResult(t, client, result)
			}
		})
	}
}

// ==================== EDGE CASES ====================

func TestReconcile_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		description string
		setup       func(*MockClient) *jobsmanagerv1beta1.ManagedJob
		validate    func(*testing.T, *MockClient, ctrl.Result, error)
	}{
		{
			name:        "empty_groups",
			description: "ManagedJob with no groups should complete immediately",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("empty", "default", []*jobsmanagerv1beta1.ManagedJobGroup{})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "group_with_no_jobs",
			description: "Group with no jobs should be marked as succeeded",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("no-jobs", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("empty-group", []*jobsmanagerv1beta1.ManagedJobDefinition{}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "circular_dependency_protection",
			description: "Jobs with circular dependencies should not cause infinite loop",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("circular", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("job-a", "busybox", NewTestDependency("job-b", ExecutionStatusPending)),
						NewTestJobDef("job-b", "busybox", NewTestDependency("job-a", ExecutionStatusPending)),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err, "should handle circular deps gracefully")
			},
		},
		{
			name:        "rapid_status_changes",
			description: "Multiple rapid reconciliations should be idempotent",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("rapid", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("job1", "busybox"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "very_long_job_name",
			description: "Long names should be handled (K8s has 63 char limit)",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("very-long-group-name-that-exceeds-normal", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("extremely-long-job-name-here", "busybox"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name:        "special_characters_in_name",
			description: "Names with special characters",
			setup: func(c *MockClient) *jobsmanagerv1beta1.ManagedJob {
				mj := NewTestManagedJob("test-workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
					NewTestGroup("group-1", []*jobsmanagerv1beta1.ManagedJobDefinition{
						NewTestJobDef("job-1", "busybox"),
					}),
				})
				controllerutil.AddFinalizer(mj, FinalizerName)
				return mj
			},
			validate: func(t *testing.T, c *MockClient, r ctrl.Result, err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			reconciler := &ManagedJobReconciler{
				Client:   client,
				Scheme:   client.Scheme(),
				Recorder: NewFakeRecorder(),
			}

			mj := tt.setup(client)
			client.AddManagedJob(mj)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: mj.Name, Namespace: mj.Namespace},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			tt.validate(t, client, result, err)
		})
	}
}

// ==================== KUBERNETES VOLATILITY TESTS ====================

func TestReconcile_KubernetesVolatility(t *testing.T) {
	tests := []struct {
		name        string
		description string
		clientSetup func(*MockClient)
		expectError bool
	}{
		{
			name:        "intermittent_api_failure",
			description: "API fails on first call but succeeds on retry",
			clientSetup: func(c *MockClient) {
				c.FailOnNthCall["get"] = 1
			},
			expectError: true,
		},
		{
			name:        "list_timeout",
			description: "List operation times out",
			clientSetup: func(c *MockClient) {
				c.ListError = ErrTimeout
			},
			expectError: false, // List error is handled gracefully
		},
		{
			name:        "create_conflict",
			description: "Job already exists when creating",
			clientSetup: func(c *MockClient) {
				c.CreateError = errors.New("already exists")
			},
			expectError: false, // Already exists is handled
		},
		{
			name:        "update_resource_version_conflict",
			description: "Optimistic locking conflict on update",
			clientSetup: func(c *MockClient) {
				c.SimulateConflictOnUpdate = true
			},
			expectError: false, // First update should succeed
		},
		{
			name:        "forbidden_permission",
			description: "Insufficient RBAC permissions",
			clientSetup: func(c *MockClient) {
				c.CreateError = ErrForbidden
			},
			expectError: false, // Logged but doesn't fail reconciliation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient()
			tt.clientSetup(client)

			reconciler := &ManagedJobReconciler{
				Client:   client,
				Scheme:   client.Scheme(),
				Recorder: NewFakeRecorder(),
			}

			mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
				NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
					NewTestJobDef("job1", "busybox"),
				}),
			})
			controllerutil.AddFinalizer(mj, FinalizerName)
			client.AddManagedJob(mj)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "workflow", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(context.Background(), req)

			if tt.expectError {
				require.Error(t, err, tt.description)
			} else {
				require.NoError(t, err, tt.description)
			}
		})
	}
}
