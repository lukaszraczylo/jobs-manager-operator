package controllers

// +kubebuilder:validation:Enum=Allow;Forbid;Replace
type ExecutionStatus string

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
