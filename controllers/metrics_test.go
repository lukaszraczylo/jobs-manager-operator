package controllers

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRecordJobCreated(t *testing.T) {
	// Reset counter for testing
	JobsCreatedTotal.Reset()

	// Record job creation
	RecordJobCreated("default", "test-workflow", "group1")
	RecordJobCreated("default", "test-workflow", "group1")
	RecordJobCreated("production", "other-workflow", "group2")

	// Verify counts
	assert.Equal(t, float64(2), testutil.ToFloat64(JobsCreatedTotal.WithLabelValues("default", "test-workflow", "group1")))
	assert.Equal(t, float64(1), testutil.ToFloat64(JobsCreatedTotal.WithLabelValues("production", "other-workflow", "group2")))
}

func TestRecordJobSucceeded(t *testing.T) {
	// Reset counter for testing
	JobsSucceededTotal.Reset()

	// Record job success
	RecordJobSucceeded("default", "workflow1", "group1")
	RecordJobSucceeded("default", "workflow1", "group1")
	RecordJobSucceeded("default", "workflow1", "group2")

	// Verify counts
	assert.Equal(t, float64(2), testutil.ToFloat64(JobsSucceededTotal.WithLabelValues("default", "workflow1", "group1")))
	assert.Equal(t, float64(1), testutil.ToFloat64(JobsSucceededTotal.WithLabelValues("default", "workflow1", "group2")))
}

func TestRecordJobFailed(t *testing.T) {
	// Reset counter for testing
	JobsFailedTotal.Reset()

	// Record job failure
	RecordJobFailed("production", "critical-workflow", "init")
	RecordJobFailed("production", "critical-workflow", "init")
	RecordJobFailed("production", "critical-workflow", "cleanup")

	// Verify counts
	assert.Equal(t, float64(2), testutil.ToFloat64(JobsFailedTotal.WithLabelValues("production", "critical-workflow", "init")))
	assert.Equal(t, float64(1), testutil.ToFloat64(JobsFailedTotal.WithLabelValues("production", "critical-workflow", "cleanup")))
}

func TestSetActiveJobs(t *testing.T) {
	// Reset gauge for testing
	ActiveJobs.Reset()

	// Set active jobs
	SetActiveJobs("default", "workflow1", 5)
	SetActiveJobs("production", "workflow2", 3)
	SetActiveJobs("default", "workflow1", 2) // Update to lower value

	// Verify values
	assert.Equal(t, float64(2), testutil.ToFloat64(ActiveJobs.WithLabelValues("default", "workflow1")))
	assert.Equal(t, float64(3), testutil.ToFloat64(ActiveJobs.WithLabelValues("production", "workflow2")))
}

func TestMetricsLabels(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		workflow  string
		group     string
		metric    *prometheus.CounterVec
		recorder  func(ns, wf, grp string)
	}{
		{
			name:      "created_metric_labels",
			namespace: "ns1",
			workflow:  "wf1",
			group:     "grp1",
			metric:    JobsCreatedTotal,
			recorder:  RecordJobCreated,
		},
		{
			name:      "succeeded_metric_labels",
			namespace: "ns2",
			workflow:  "wf2",
			group:     "grp2",
			metric:    JobsSucceededTotal,
			recorder:  RecordJobSucceeded,
		},
		{
			name:      "failed_metric_labels",
			namespace: "ns3",
			workflow:  "wf3",
			group:     "grp3",
			metric:    JobsFailedTotal,
			recorder:  RecordJobFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.metric.Reset()
			tt.recorder(tt.namespace, tt.workflow, tt.group)

			value := testutil.ToFloat64(tt.metric.WithLabelValues(tt.namespace, tt.workflow, tt.group))
			assert.Equal(t, float64(1), value)
		})
	}
}

func TestMetricsMultipleNamespaces(t *testing.T) {
	// Reset all metrics
	JobsCreatedTotal.Reset()
	JobsSucceededTotal.Reset()
	JobsFailedTotal.Reset()
	ActiveJobs.Reset()

	namespaces := []string{"dev", "staging", "production"}

	for _, ns := range namespaces {
		RecordJobCreated(ns, "workflow", "group")
		RecordJobSucceeded(ns, "workflow", "group")
		RecordJobFailed(ns, "workflow", "group")
		SetActiveJobs(ns, "workflow", 1)
	}

	for _, ns := range namespaces {
		assert.Equal(t, float64(1), testutil.ToFloat64(JobsCreatedTotal.WithLabelValues(ns, "workflow", "group")))
		assert.Equal(t, float64(1), testutil.ToFloat64(JobsSucceededTotal.WithLabelValues(ns, "workflow", "group")))
		assert.Equal(t, float64(1), testutil.ToFloat64(JobsFailedTotal.WithLabelValues(ns, "workflow", "group")))
		assert.Equal(t, float64(1), testutil.ToFloat64(ActiveJobs.WithLabelValues(ns, "workflow")))
	}
}

func TestActiveJobsGaugeDecreases(t *testing.T) {
	ActiveJobs.Reset()

	// Simulate job lifecycle
	SetActiveJobs("default", "workflow", 0)
	assert.Equal(t, float64(0), testutil.ToFloat64(ActiveJobs.WithLabelValues("default", "workflow")))

	SetActiveJobs("default", "workflow", 5)
	assert.Equal(t, float64(5), testutil.ToFloat64(ActiveJobs.WithLabelValues("default", "workflow")))

	SetActiveJobs("default", "workflow", 3)
	assert.Equal(t, float64(3), testutil.ToFloat64(ActiveJobs.WithLabelValues("default", "workflow")))

	SetActiveJobs("default", "workflow", 0)
	assert.Equal(t, float64(0), testutil.ToFloat64(ActiveJobs.WithLabelValues("default", "workflow")))
}
