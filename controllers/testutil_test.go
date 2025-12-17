package controllers

import (
	"context"
	"errors"
	"sync"

	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

// Common test errors for simulating Kubernetes API failures
var (
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict: object has been modified")
	ErrTimeout        = errors.New("context deadline exceeded")
	ErrServerError    = errors.New("internal server error")
	ErrForbidden      = errors.New("forbidden: insufficient permissions")
	ErrNetworkFailure = errors.New("network is unreachable")
)

// TestScenario defines the type of test scenario
type TestScenario string

const (
	ScenarioGood      TestScenario = "good"
	ScenarioNotGood   TestScenario = "not_good"
	ScenarioReallyBad TestScenario = "really_bad"
)

// MockClient implements a mock Kubernetes client for testing
type MockClient struct {
	mu sync.RWMutex

	// Storage for mock objects
	managedJobs map[string]*jobsmanagerv1beta1.ManagedJob
	jobs        map[string]*kbatch.Job

	// Error injection for different operations
	GetError    error
	ListError   error
	CreateError error
	UpdateError error
	DeleteError error

	// Call counters for verification
	GetCalls    int
	ListCalls   int
	CreateCalls int
	UpdateCalls int
	DeleteCalls int

	// Behavior modifiers
	SimulateConflictOnUpdate bool
	SimulateSlowResponse     bool
	FailOnNthCall            map[string]int // operation -> fail on nth call
}

// NewMockClient creates a new mock client
func NewMockClient() *MockClient {
	return &MockClient{
		managedJobs:   make(map[string]*jobsmanagerv1beta1.ManagedJob),
		jobs:          make(map[string]*kbatch.Job),
		FailOnNthCall: make(map[string]int),
	}
}

// Get implements client.Client
func (m *MockClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.GetCalls++

	// Check for context cancellation (simulates timeout)
	select {
	case <-ctx.Done():
		return ErrTimeout
	default:
	}

	// Check for injected error
	if m.GetError != nil {
		return m.GetError
	}

	// Check for nth call failure
	if n, ok := m.FailOnNthCall["get"]; ok && m.GetCalls == n {
		return ErrServerError
	}

	keyStr := key.String()

	switch v := obj.(type) {
	case *jobsmanagerv1beta1.ManagedJob:
		if mj, ok := m.managedJobs[keyStr]; ok {
			*v = *mj.DeepCopy()
			return nil
		}
		return apierrors.NewNotFound(schema.GroupResource{Group: "jobsmanager.raczylo.com", Resource: "managedjobs"}, key.Name)
	case *kbatch.Job:
		if j, ok := m.jobs[keyStr]; ok {
			*v = *j.DeepCopy()
			return nil
		}
		return apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, key.Name)
	}

	return apierrors.NewNotFound(schema.GroupResource{Resource: "unknown"}, key.Name)
}

// List implements client.Client
func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ListCalls++

	select {
	case <-ctx.Done():
		return ErrTimeout
	default:
	}

	if m.ListError != nil {
		return m.ListError
	}

	if n, ok := m.FailOnNthCall["list"]; ok && m.ListCalls == n {
		return ErrServerError
	}

	// Extract namespace from options
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	switch v := list.(type) {
	case *kbatch.JobList:
		items := []kbatch.Job{}
		for _, job := range m.jobs {
			if listOpts.Namespace == "" || job.Namespace == listOpts.Namespace {
				// Check label selector if present
				if listOpts.LabelSelector != nil {
					if !listOpts.LabelSelector.Matches(labelSetFromMap(job.Labels)) {
						continue
					}
				}
				items = append(items, *job.DeepCopy())
			}
		}
		v.Items = items
		return nil
	case *jobsmanagerv1beta1.ManagedJobList:
		items := []jobsmanagerv1beta1.ManagedJob{}
		for _, mj := range m.managedJobs {
			if listOpts.Namespace == "" || mj.Namespace == listOpts.Namespace {
				items = append(items, *mj.DeepCopy())
			}
		}
		v.Items = items
		return nil
	}

	return nil
}

// Create implements client.Client
func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.CreateCalls++

	select {
	case <-ctx.Done():
		return ErrTimeout
	default:
	}

	if m.CreateError != nil {
		return m.CreateError
	}

	if n, ok := m.FailOnNthCall["create"]; ok && m.CreateCalls == n {
		return ErrServerError
	}

	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}.String()

	switch v := obj.(type) {
	case *jobsmanagerv1beta1.ManagedJob:
		if _, exists := m.managedJobs[key]; exists {
			return errors.New("already exists")
		}
		m.managedJobs[key] = v.DeepCopy()
	case *kbatch.Job:
		if _, exists := m.jobs[key]; exists {
			return errors.New("already exists")
		}
		m.jobs[key] = v.DeepCopy()
	}

	return nil
}

// Update implements client.Client
func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.UpdateCalls++

	select {
	case <-ctx.Done():
		return ErrTimeout
	default:
	}

	if m.UpdateError != nil {
		return m.UpdateError
	}

	if m.SimulateConflictOnUpdate && m.UpdateCalls > 1 {
		return ErrConflict
	}

	if n, ok := m.FailOnNthCall["update"]; ok && m.UpdateCalls == n {
		return ErrServerError
	}

	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}.String()

	switch v := obj.(type) {
	case *jobsmanagerv1beta1.ManagedJob:
		if _, exists := m.managedJobs[key]; !exists {
			return ErrNotFound
		}
		m.managedJobs[key] = v.DeepCopy()
	case *kbatch.Job:
		if _, exists := m.jobs[key]; !exists {
			return ErrNotFound
		}
		m.jobs[key] = v.DeepCopy()
	}

	return nil
}

// Delete implements client.Client
func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeleteCalls++

	select {
	case <-ctx.Done():
		return ErrTimeout
	default:
	}

	if m.DeleteError != nil {
		return m.DeleteError
	}

	if n, ok := m.FailOnNthCall["delete"]; ok && m.DeleteCalls == n {
		return ErrServerError
	}

	key := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}.String()

	switch obj.(type) {
	case *jobsmanagerv1beta1.ManagedJob:
		delete(m.managedJobs, key)
	case *kbatch.Job:
		delete(m.jobs, key)
	}

	return nil
}

// Patch implements client.Client
func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.Update(ctx, obj)
}

// DeleteAllOf implements client.Client
func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}

// Status implements client.Client
func (m *MockClient) Status() client.SubResourceWriter {
	return &MockStatusWriter{client: m}
}

// SubResource implements client.Client
func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	return nil
}

// Apply implements client.Client
func (m *MockClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return nil
}

// Scheme implements client.Client
func (m *MockClient) Scheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = jobsmanagerv1beta1.AddToScheme(scheme)
	_ = kbatch.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

// RESTMapper implements client.Client
func (m *MockClient) RESTMapper() meta.RESTMapper {
	return nil
}

// GroupVersionKindFor implements client.Client
func (m *MockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

// IsObjectNamespaced implements client.Client
func (m *MockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

// MockStatusWriter implements client.StatusWriter
type MockStatusWriter struct {
	client *MockClient
}

func (m *MockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return nil
}

func (m *MockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return m.client.Update(ctx, obj)
}

func (m *MockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return m.client.Patch(ctx, obj, patch)
}

// Helper to add test data
func (m *MockClient) AddManagedJob(mj *jobsmanagerv1beta1.ManagedJob) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := types.NamespacedName{
		Namespace: mj.Namespace,
		Name:      mj.Name,
	}.String()
	m.managedJobs[key] = mj.DeepCopy()
}

func (m *MockClient) AddJob(job *kbatch.Job) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := types.NamespacedName{
		Namespace: job.Namespace,
		Name:      job.Name,
	}.String()
	m.jobs[key] = job.DeepCopy()
}

func (m *MockClient) GetManagedJobByKey(key string) *jobsmanagerv1beta1.ManagedJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.managedJobs[key]
}

func (m *MockClient) GetJobByKey(key string) *kbatch.Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.jobs[key]
}

// labelSetFromMap creates a label set from a map for selector matching
type labelSet map[string]string

func labelSetFromMap(m map[string]string) labelSet {
	return labelSet(m)
}

func (ls labelSet) Has(key string) bool {
	_, ok := ls[key]
	return ok
}

func (ls labelSet) Get(key string) string {
	return ls[key]
}

// Lookup implements labels.Labels interface
func (ls labelSet) Lookup(key string) (string, bool) {
	v, ok := ls[key]
	return v, ok
}

// Test fixtures and factory functions

// NewTestManagedJob creates a ManagedJob for testing
func NewTestManagedJob(name, namespace string, groups []*jobsmanagerv1beta1.ManagedJobGroup) *jobsmanagerv1beta1.ManagedJob {
	return &jobsmanagerv1beta1.ManagedJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("test-uid-" + name),
		},
		Spec: jobsmanagerv1beta1.ManagedJobSpec{
			Groups: groups,
		},
		Status: ExecutionStatusPending,
	}
}

// NewTestGroup creates a ManagedJobGroup for testing
func NewTestGroup(name string, jobs []*jobsmanagerv1beta1.ManagedJobDefinition, deps ...*jobsmanagerv1beta1.ManagedJobDependencies) *jobsmanagerv1beta1.ManagedJobGroup {
	return &jobsmanagerv1beta1.ManagedJobGroup{
		Name:         name,
		Jobs:         jobs,
		Dependencies: deps,
		Status:       ExecutionStatusPending,
	}
}

// NewTestJobDef creates a ManagedJobDefinition for testing
func NewTestJobDef(name, image string, deps ...*jobsmanagerv1beta1.ManagedJobDependencies) *jobsmanagerv1beta1.ManagedJobDefinition {
	return &jobsmanagerv1beta1.ManagedJobDefinition{
		Name:         name,
		Image:        image,
		Args:         []string{"echo", "test"},
		Dependencies: deps,
		Status:       ExecutionStatusPending,
	}
}

// NewTestDependency creates a ManagedJobDependencies for testing
func NewTestDependency(name, status string) *jobsmanagerv1beta1.ManagedJobDependencies {
	return &jobsmanagerv1beta1.ManagedJobDependencies{
		Name:   name,
		Status: status,
	}
}

// NewTestK8sJob creates a Kubernetes Job for testing
func NewTestK8sJob(name, namespace, workflowName, groupName string, status kbatch.JobStatus) *kbatch.Job {
	return &kbatch.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				LabelWorkflowName: workflowName,
				LabelGroupName:    groupName,
				LabelJobName:      name,
			},
		},
		Spec: kbatch.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "test", Image: "busybox"},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
		Status: status,
	}
}

// NewFakeRecorder creates a fake event recorder for testing
func NewFakeRecorder() record.EventRecorder {
	return record.NewFakeRecorder(100)
}
