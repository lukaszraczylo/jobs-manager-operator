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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

func TestJobNameGenerator(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "single part",
			parts:    []string{"workflow"},
			expected: "workflow",
		},
		{
			name:     "multiple parts",
			parts:    []string{"workflow", "group", "job"},
			expected: "workflow-group-job",
		},
		{
			name:     "uppercase conversion",
			parts:    []string{"Workflow", "GROUP", "Job"},
			expected: "workflow-group-job",
		},
		{
			name:     "mixed case",
			parts:    []string{"MyWorkflow", "TestGroup", "BuildJob"},
			expected: "myworkflow-testgroup-buildjob",
		},
		{
			name:     "empty parts",
			parts:    []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jobNameGenerator(tt.parts...)
			if result != tt.expected {
				t.Errorf("jobNameGenerator(%v) = %v, want %v", tt.parts, result, tt.expected)
			}
		})
	}
}

func TestGetImagePullPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   string
		expected corev1.PullPolicy
	}{
		{
			name:     "empty policy defaults to IfNotPresent",
			policy:   "",
			expected: corev1.PullIfNotPresent,
		},
		{
			name:     "Always policy",
			policy:   "Always",
			expected: corev1.PullAlways,
		},
		{
			name:     "Never policy",
			policy:   "Never",
			expected: corev1.PullNever,
		},
		{
			name:     "IfNotPresent policy",
			policy:   "IfNotPresent",
			expected: corev1.PullIfNotPresent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getImagePullPolicy(tt.policy)
			if result != tt.expected {
				t.Errorf("getImagePullPolicy(%v) = %v, want %v", tt.policy, result, tt.expected)
			}
		})
	}
}

func TestGetResources(t *testing.T) {
	tests := []struct {
		name      string
		resources *corev1.ResourceRequirements
		expectNil bool
	}{
		{
			name:      "nil resources returns empty",
			resources: nil,
			expectNil: false,
		},
		{
			name: "non-nil resources returned as-is",
			resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
			expectNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getResources(tt.resources)
			if tt.resources == nil {
				// Should return empty struct
				if result.Limits != nil || result.Requests != nil {
					t.Errorf("getResources(nil) should return empty ResourceRequirements")
				}
			} else {
				// Should return the same values
				if len(result.Limits) != len(tt.resources.Limits) {
					t.Errorf("getResources() limits mismatch")
				}
			}
		})
	}
}

func TestBuildDependencyMaps(t *testing.T) {
	// Create a mock ManagedJob with dependencies
	mj := &jobsmanagerv1beta1.ManagedJob{
		Spec: jobsmanagerv1beta1.ManagedJobSpec{
			Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
				{
					Name: "group1",
					Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
						{Name: "init-group", Status: "pending"},
					},
					Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
						{
							Name: "job1",
							Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
								{Name: "setup-job", Status: "pending"},
							},
						},
						{
							Name: "job2",
							Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
								{Name: "setup-job", Status: "pending"},
								{Name: "job1", Status: "pending"},
							},
						},
					},
				},
				{
					Name: "group2",
					Dependencies: []*jobsmanagerv1beta1.ManagedJobDependencies{
						{Name: "group1", Status: "pending"},
					},
					Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
						{
							Name:         "job3",
							Dependencies: nil,
						},
					},
				},
			},
		},
	}

	cp := &connPackage{mj: mj}
	cp.buildDependencyMaps()

	// Verify job dependency map
	t.Run("job dependency map - setup-job has 2 dependents", func(t *testing.T) {
		deps, exists := cp.jobDepMap["setup-job"]
		if !exists {
			t.Fatal("setup-job should exist in jobDepMap")
		}
		if len(deps) != 2 {
			t.Errorf("setup-job should have 2 dependents, got %d", len(deps))
		}
	})

	t.Run("job dependency map - job1 has 1 dependent", func(t *testing.T) {
		deps, exists := cp.jobDepMap["job1"]
		if !exists {
			t.Fatal("job1 should exist in jobDepMap")
		}
		if len(deps) != 1 {
			t.Errorf("job1 should have 1 dependent, got %d", len(deps))
		}
	})

	t.Run("job dependency map - non-existent job", func(t *testing.T) {
		_, exists := cp.jobDepMap["non-existent"]
		if exists {
			t.Error("non-existent job should not be in jobDepMap")
		}
	})

	// Verify group dependency map
	t.Run("group dependency map - init-group has 1 dependent", func(t *testing.T) {
		deps, exists := cp.groupDepMap["init-group"]
		if !exists {
			t.Fatal("init-group should exist in groupDepMap")
		}
		if len(deps) != 1 {
			t.Errorf("init-group should have 1 dependent, got %d", len(deps))
		}
	})

	t.Run("group dependency map - group1 has 1 dependent", func(t *testing.T) {
		deps, exists := cp.groupDepMap["group1"]
		if !exists {
			t.Fatal("group1 should exist in groupDepMap")
		}
		if len(deps) != 1 {
			t.Errorf("group1 should have 1 dependent, got %d", len(deps))
		}
	})
}

func TestUpdateDependentJobs(t *testing.T) {
	dep1 := &jobsmanagerv1beta1.ManagedJobDependencies{Name: "target-job", Status: "pending"}
	dep2 := &jobsmanagerv1beta1.ManagedJobDependencies{Name: "target-job", Status: "pending"}
	dep3 := &jobsmanagerv1beta1.ManagedJobDependencies{Name: "other-job", Status: "pending"}

	cp := &connPackage{
		jobDepMap: map[string][]*jobsmanagerv1beta1.ManagedJobDependencies{
			"target-job": {dep1, dep2},
			"other-job":  {dep3},
		},
	}

	// Update target-job status
	cp.updateDependentJobs("target-job", "succeeded")

	t.Run("target-job dependents updated", func(t *testing.T) {
		if dep1.Status != "succeeded" {
			t.Errorf("dep1.Status = %v, want succeeded", dep1.Status)
		}
		if dep2.Status != "succeeded" {
			t.Errorf("dep2.Status = %v, want succeeded", dep2.Status)
		}
	})

	t.Run("other-job dependents unchanged", func(t *testing.T) {
		if dep3.Status != "pending" {
			t.Errorf("dep3.Status = %v, want pending (should be unchanged)", dep3.Status)
		}
	})

	t.Run("non-existent job is safe", func(t *testing.T) {
		// Should not panic
		cp.updateDependentJobs("non-existent-job", "succeeded")
	})
}

func TestUpdateDependentGroups(t *testing.T) {
	dep1 := &jobsmanagerv1beta1.ManagedJobDependencies{Name: "target-group", Status: "pending"}
	dep2 := &jobsmanagerv1beta1.ManagedJobDependencies{Name: "other-group", Status: "pending"}

	cp := &connPackage{
		groupDepMap: map[string][]*jobsmanagerv1beta1.ManagedJobDependencies{
			"target-group": {dep1},
			"other-group":  {dep2},
		},
	}

	cp.updateDependentGroups("target-group", "succeeded")

	t.Run("target-group dependents updated", func(t *testing.T) {
		if dep1.Status != "succeeded" {
			t.Errorf("dep1.Status = %v, want succeeded", dep1.Status)
		}
	})

	t.Run("other-group dependents unchanged", func(t *testing.T) {
		if dep2.Status != "pending" {
			t.Errorf("dep2.Status = %v, want pending (should be unchanged)", dep2.Status)
		}
	})
}

func TestCompileParameters(t *testing.T) {
	cp := &connPackage{}

	t.Run("merges multiple parameter sets", func(t *testing.T) {
		params1 := jobsmanagerv1beta1.ManagedJobParameters{
			ServiceAccount: "sa1",
			RestartPolicy:  "Never",
		}
		params2 := jobsmanagerv1beta1.ManagedJobParameters{
			ServiceAccount:  "sa2", // should override
			ImagePullPolicy: "Always",
		}

		result := cp.compileParameters(params1, params2)

		if result.ServiceAccount != "sa2" {
			t.Errorf("ServiceAccount = %v, want sa2", result.ServiceAccount)
		}
		if result.RestartPolicy != "Never" {
			t.Errorf("RestartPolicy = %v, want Never", result.RestartPolicy)
		}
		if result.ImagePullPolicy != "Always" {
			t.Errorf("ImagePullPolicy = %v, want Always", result.ImagePullPolicy)
		}
	})

	t.Run("merges env vars", func(t *testing.T) {
		params1 := jobsmanagerv1beta1.ManagedJobParameters{
			Env: []corev1.EnvVar{{Name: "VAR1", Value: "val1"}},
		}
		params2 := jobsmanagerv1beta1.ManagedJobParameters{
			Env: []corev1.EnvVar{{Name: "VAR2", Value: "val2"}},
		}

		result := cp.compileParameters(params1, params2)

		if len(result.Env) != 2 {
			t.Errorf("len(Env) = %v, want 2", len(result.Env))
		}
	})

	t.Run("handles empty parameters", func(t *testing.T) {
		result := cp.compileParameters(jobsmanagerv1beta1.ManagedJobParameters{})
		if result.ServiceAccount != "" {
			t.Errorf("ServiceAccount should be empty")
		}
	})
}
