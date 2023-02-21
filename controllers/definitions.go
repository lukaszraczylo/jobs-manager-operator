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

var (
	jobOwnerKey = ".metadata.controller"
)

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
