package controllers

// +kubebuilder:validation:Enum=Allow;Forbid;Replace
const (
	ExecutionStatusPending   string = "pending"
	ExecutionStatusRunning   string = "running"
	ExecutionStatusSucceeded string = "succeeded"
	ExecutionStatusFailed    string = "failed"
	ExecutionStatusAborted   string = "aborted"
	ExecutionStatusUnknown   string = "unknown"
)

// Label keys used for job tracking and identification
const (
	LabelWorkflowName = "jobmanager.raczylo.com/workflow-name"
	LabelGroupName    = "jobmanager.raczylo.com/group-name"
	LabelJobName      = "jobmanager.raczylo.com/job-name"
	LabelJobID        = "jobmanager.raczylo.com/job-id"
)

// FinalizerName is the finalizer used to ensure cleanup of child resources
const FinalizerName = "jobmanager.raczylo.com/finalizer"

type (
	ExecutionStatus string

	tree struct {
		text  string
		items []Tree
	}

	// Tree is tree interface
	Tree interface {
		Add(text string) Tree
		AddTree(tree Tree)
		Items() []Tree
		Text() string
		Print() string
	}

	printer struct {
	}

	// Printer is printer interface
	Printer interface {
		Print(Tree) string
	}
)
