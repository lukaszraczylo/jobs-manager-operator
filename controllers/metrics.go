package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// JobsCreatedTotal tracks the total number of jobs created
	JobsCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "managedjob_jobs_created_total",
			Help: "Total number of Kubernetes jobs created by the operator",
		},
		[]string{"namespace", "workflow", "group"},
	)

	// JobsSucceededTotal tracks the total number of jobs that succeeded
	JobsSucceededTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "managedjob_jobs_succeeded_total",
			Help: "Total number of jobs that completed successfully",
		},
		[]string{"namespace", "workflow", "group"},
	)

	// JobsFailedTotal tracks the total number of jobs that failed
	JobsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "managedjob_jobs_failed_total",
			Help: "Total number of jobs that failed",
		},
		[]string{"namespace", "workflow", "group"},
	)

	// ReconciliationDuration tracks how long reconciliations take
	ReconciliationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "managedjob_reconciliation_duration_seconds",
			Help:    "Time spent reconciling ManagedJob resources",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"namespace", "workflow"},
	)

	// ActiveJobs tracks the number of currently running jobs per workflow
	ActiveJobs = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "managedjob_active_jobs",
			Help: "Number of currently active (running) jobs per workflow",
		},
		[]string{"namespace", "workflow"},
	)
)

func init() {
	// Register custom metrics with the controller-runtime metrics registry
	metrics.Registry.MustRegister(
		JobsCreatedTotal,
		JobsSucceededTotal,
		JobsFailedTotal,
		ReconciliationDuration,
		ActiveJobs,
	)
}

// RecordJobCreated increments the job created counter
func RecordJobCreated(namespace, workflow, group string) {
	JobsCreatedTotal.WithLabelValues(namespace, workflow, group).Inc()
}

// RecordJobSucceeded increments the job succeeded counter
func RecordJobSucceeded(namespace, workflow, group string) {
	JobsSucceededTotal.WithLabelValues(namespace, workflow, group).Inc()
}

// RecordJobFailed increments the job failed counter
func RecordJobFailed(namespace, workflow, group string) {
	JobsFailedTotal.WithLabelValues(namespace, workflow, group).Inc()
}

// SetActiveJobs sets the number of active jobs for a workflow
func SetActiveJobs(namespace, workflow string, count float64) {
	ActiveJobs.WithLabelValues(namespace, workflow).Set(count)
}
