package visualization

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

// Client wraps the Kubernetes client for ManagedJob operations
type Client struct {
	client client.Client
}

// NewClient creates a new Client for ManagedJob operations
func NewClient() (*Client, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	if err := jobsmanagerv1beta1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add jobsmanager scheme: %w", err)
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Client{client: cl}, nil
}

// GetManagedJob retrieves a ManagedJob by name and namespace
func (c *Client) GetManagedJob(ctx context.Context, name, namespace string) (*jobsmanagerv1beta1.ManagedJob, error) {
	mj := &jobsmanagerv1beta1.ManagedJob{}
	err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, mj)
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedJob %s/%s: %w", namespace, name, err)
	}
	return mj, nil
}

// ListManagedJobs lists all ManagedJobs in a namespace
func (c *Client) ListManagedJobs(ctx context.Context, namespace string) (*jobsmanagerv1beta1.ManagedJobList, error) {
	mjList := &jobsmanagerv1beta1.ManagedJobList{}
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	err := c.client.List(ctx, mjList, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to list ManagedJobs: %w", err)
	}
	return mjList, nil
}

// BuildTree builds a StatusTree from a ManagedJob
func BuildTree(mj *jobsmanagerv1beta1.ManagedJob) *StatusTree {
	root := NewStatusTreeWithStatus(mj.Name, mj.Status)

	for _, group := range mj.Spec.Groups {
		groupNode := root.AddWithStatus(group.Name, group.Status)

		// Add group dependencies
		for _, dep := range group.Dependencies {
			groupNode.Add(RenderDependency(dep.Name, true))
		}

		// Add jobs
		for _, job := range group.Jobs {
			jobNode := groupNode.AddWithStatus(job.Name, job.Status)

			// Add job dependencies
			for _, dep := range job.Dependencies {
				jobNode.Add(RenderDependency(dep.Name, false))
			}
		}
	}

	return root
}

// GetStatusSummary returns a summary of job statuses
type StatusSummary struct {
	Name      string
	Namespace string
	Status    string
	Groups    int
	Jobs      int
	Pending   int
	Running   int
	Succeeded int
	Failed    int
	Aborted   int
}

// GetStatusSummary builds a summary of the ManagedJob status
func GetStatusSummary(mj *jobsmanagerv1beta1.ManagedJob) StatusSummary {
	summary := StatusSummary{
		Name:      mj.Name,
		Namespace: mj.Namespace,
		Status:    mj.Status,
		Groups:    len(mj.Spec.Groups),
	}

	for _, group := range mj.Spec.Groups {
		for _, job := range group.Jobs {
			summary.Jobs++
			switch job.Status {
			case StatusPending:
				summary.Pending++
			case StatusRunning:
				summary.Running++
			case StatusSucceeded:
				summary.Succeeded++
			case StatusFailed:
				summary.Failed++
			case StatusAborted:
				summary.Aborted++
			}
		}
	}

	return summary
}
