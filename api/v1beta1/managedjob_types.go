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

type ManagedJobDependencies struct {
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ManagedJobDefinition struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=[a-z0-9-]+
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Parallel bool `json:"parallel"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=5
	Image string `json:"image"`
	// +kubebuilder:validation:Optional
	Args []string `json:"args,omitempty"`
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
	Status string `json:"status"`
	// +kubebuilder:validation:Optional
	// +optional
	Dependencies []*ManagedJobDependencies `json:"dependencies"`
	// +optional
	CompiledParams ManagedJobParameters `json:"compiledParams"`
}

type ManagedJobGroup struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=40
	// +kubebuilder:validation:Pattern=[a-z0-9-]+
	Name string `json:"name"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Parallel bool `json:"parallel"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Jobs []*ManagedJobDefinition `json:"jobs"`
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
	// +kubebuilder:validation:Optional
	// +optional
	Dependencies []*ManagedJobDependencies `json:"dependencies"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
	Status string `json:"status"`
}

type ManagedJobParameters struct {
	// +kubebuilder:validation:Optional
	FromEnv []corev1.EnvFromSource `json:"fromEnv,omitempty"`
	// +kubebuilder:validation:Optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// +kubebuilder:validation:Optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// +kubebuilder:validation:Optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMount,omitempty"`
	// +kubebuilder:validation:Optional
	ServiceAccount string `json:"serviceAccount,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=OnFailure
	RestartPolicy string `json:"restartPolicy,omitempty"`
	// +kubebuilder:validation:Optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// +kubebuilder:validation:Optional
	ImagePullPolicy string `json:"imagePullPolicy,omitempty"`
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ManagedJobSpec defines the desired state of ManagedJob
type ManagedJobSpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	Retries int `json:"retries"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Groups []*ManagedJobGroup `json:"groups"`
	// +kubebuilder:validation:Optional
	Params ManagedJobParameters `json:"params"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// ManagedJob is the Schema for the managedjobs API
type ManagedJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ManagedJobSpec `json:"spec,omitempty"`
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=pending
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
