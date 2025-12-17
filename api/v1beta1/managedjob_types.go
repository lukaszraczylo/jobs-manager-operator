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

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ManagedJobDependencies defines a dependency relationship between jobs or groups
type ManagedJobDependencies struct {
	// Name is the identifier of the dependency (job or group name)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	Name string `json:"name"`
	// Status tracks the execution status of the dependency
	// +kubebuilder:validation:Enum=pending;running;succeeded;failed;aborted
	Status string `json:"status"`
}

// ManagedJobDefinition defines a single job within a group
type ManagedJobDefinition struct {
	// Name is the unique identifier for this job within the group
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=[a-z0-9-]+
	Name string `json:"name"`
	// Parallel indicates if this job can run in parallel with others in the group
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Parallel bool `json:"parallel"`
	// Image is the container image to run for this job
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=5
	Image string `json:"image"`
	// Args are the command-line arguments to pass to the container
	// +kubebuilder:validation:Optional
	Args []string `json:"args,omitempty"`
	// Params contains job-specific parameters that override group and spec-level params
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
	// Status tracks the execution status of this job
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
	// +kubebuilder:validation:Enum=pending;running;succeeded;failed;aborted
	Status string `json:"status"`
	// Dependencies lists the jobs that must complete before this job can run
	// +kubebuilder:validation:Optional
	// +optional
	Dependencies []*ManagedJobDependencies `json:"dependencies"`
	// CompiledParams contains the merged parameters from spec, group, and job levels
	// +optional
	CompiledParams ManagedJobParameters `json:"compiledParams"`
}

// ManagedJobGroup defines a group of jobs that can be executed together
type ManagedJobGroup struct {
	// Name is the unique identifier for this group within the workflow
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=[a-z0-9-]+
	Name string `json:"name"`
	// Parallel indicates if this group can run in parallel with other groups
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Parallel bool `json:"parallel"`
	// Jobs is the list of jobs to execute within this group
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Jobs []*ManagedJobDefinition `json:"jobs"`
	// Params contains group-level parameters that override spec-level params
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
	// Dependencies lists the groups that must complete before this group can run
	// +kubebuilder:validation:Optional
	// +optional
	Dependencies []*ManagedJobDependencies `json:"dependencies"`
	// Status tracks the execution status of this group
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
	// +kubebuilder:validation:Enum=pending;running;succeeded;failed;aborted
	Status string `json:"status"`
}

// ManagedJobParameters defines common parameters that can be set at spec, group, or job level.
// Parameters at lower levels override those at higher levels.
type ManagedJobParameters struct {
	// FromEnv specifies environment variable sources (ConfigMaps, Secrets)
	// +kubebuilder:validation:Optional
	FromEnv []corev1.EnvFromSource `json:"fromEnv,omitempty"`
	// Env specifies individual environment variables
	// +kubebuilder:validation:Optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Volumes specifies volumes to mount in job pods
	// +kubebuilder:validation:Optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// VolumeMounts specifies where to mount volumes in containers
	// +kubebuilder:validation:Optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMount,omitempty"`
	// ServiceAccount is the Kubernetes service account to use for job pods
	// +kubebuilder:validation:Optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
	// RestartPolicy defines the pod restart policy (Never, OnFailure)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=OnFailure
	// +kubebuilder:validation:Enum=Never;OnFailure
	RestartPolicy string `json:"restartPolicy,omitempty"`
	// ImagePullSecrets are references to secrets for pulling private images
	// +kubebuilder:validation:Optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// ImagePullPolicy defines when to pull the container image
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`
	// Labels are additional labels to apply to job pods
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
	// Annotations are additional annotations to apply to job pods
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Resources specifies compute resources for the job container
	// +kubebuilder:validation:Optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ManagedJobSpec defines the desired state of ManagedJob
type ManagedJobSpec struct {
	// Retries is the number of times to retry failed jobs
	// +kubebuilder:validation:Required
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Retries int `json:"retries"`
	// Groups is the list of job groups to execute in this workflow
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Groups []*ManagedJobGroup `json:"groups"`
	// Params contains spec-level parameters that apply to all jobs
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ManagedJob is the Schema for the managedjobs API.
// It defines a workflow consisting of groups of jobs with dependencies.
type ManagedJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired workflow configuration
	Spec ManagedJobSpec `json:"spec,omitempty"`
	// Status tracks the overall execution status of the workflow
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
	// +kubebuilder:validation:Enum=pending;running;succeeded;failed;aborted
	Status string `json:"status"`
}

//+kubebuilder:object:root=true

// ManagedJobList contains a list of ManagedJob
type ManagedJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedJob{}, &ManagedJobList{})
}
