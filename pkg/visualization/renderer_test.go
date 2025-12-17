package visualization

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type RendererTestSuite struct {
	suite.Suite
}

func TestRendererSuite(t *testing.T) {
	suite.Run(t, new(RendererTestSuite))
}

func (s *RendererTestSuite) TestNewRenderer() {
	r := NewRenderer(true)
	s.NotNil(r)
	s.True(r.useColor)

	r2 := NewRenderer(false)
	s.NotNil(r2)
	s.False(r2.useColor)
}

func (s *RendererTestSuite) TestRender_SimpleTree() {
	r := NewRenderer(false)
	tree := NewStatusTree("root")

	output := r.Render(tree)

	s.Contains(output, "root")
}

func (s *RendererTestSuite) TestRender_WithStatus() {
	r := NewRenderer(false)
	tree := NewStatusTreeWithStatus("workflow", StatusRunning)

	output := r.Render(tree)

	s.Contains(output, "workflow")
	s.Contains(output, "[running]")
}

func (s *RendererTestSuite) TestRender_WithChildren() {
	r := NewRenderer(false)
	tree := NewStatusTree("root")
	tree.Add("child1")
	tree.Add("child2")

	output := r.Render(tree)

	s.Contains(output, "root")
	s.Contains(output, "child1")
	s.Contains(output, "child2")
	// Check for tree characters
	s.Contains(output, "├──")
	s.Contains(output, "└──")
}

func (s *RendererTestSuite) TestRender_NestedChildren() {
	r := NewRenderer(false)
	tree := NewStatusTree("workflow")
	group := tree.Add("group1")
	group.Add("job1")
	group.Add("job2")

	output := r.Render(tree)

	s.Contains(output, "workflow")
	s.Contains(output, "group1")
	s.Contains(output, "job1")
	s.Contains(output, "job2")
}

func (s *RendererTestSuite) TestRender_AllStatusColors() {
	r := NewRenderer(false)
	tree := NewStatusTree("workflow")
	tree.AddWithStatus("pending-job", StatusPending)
	tree.AddWithStatus("running-job", StatusRunning)
	tree.AddWithStatus("succeeded-job", StatusSucceeded)
	tree.AddWithStatus("failed-job", StatusFailed)
	tree.AddWithStatus("aborted-job", StatusAborted)
	tree.AddWithStatus("unknown-job", StatusUnknown)

	output := r.Render(tree)

	s.Contains(output, "[pending]")
	s.Contains(output, "[running]")
	s.Contains(output, "[succeeded]")
	s.Contains(output, "[failed]")
	s.Contains(output, "[aborted]")
	s.Contains(output, "[unknown]")
}

func (s *RendererTestSuite) TestRenderDependency_Job() {
	result := RenderDependency("init-job", false)
	s.Equal("depends on: init-job", result)
}

func (s *RendererTestSuite) TestRenderDependency_Group() {
	result := RenderDependency("init-group", true)
	s.Equal("depends on group: init-group", result)
}

// ==================== MATRIX TEST: RENDER SCENARIOS ====================

func TestRenderer_StatusRendering(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{
			name:     "pending_status",
			status:   StatusPending,
			expected: "[pending]",
		},
		{
			name:     "running_status",
			status:   StatusRunning,
			expected: "[running]",
		},
		{
			name:     "succeeded_status",
			status:   StatusSucceeded,
			expected: "[succeeded]",
		},
		{
			name:     "failed_status",
			status:   StatusFailed,
			expected: "[failed]",
		},
		{
			name:     "aborted_status",
			status:   StatusAborted,
			expected: "[aborted]",
		},
		{
			name:     "unknown_status",
			status:   StatusUnknown,
			expected: "[unknown]",
		},
		{
			name:     "custom_status",
			status:   "custom",
			expected: "[custom]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer(false)
			tree := NewStatusTreeWithStatus("node", tt.status)

			output := r.Render(tree)

			assert.Contains(t, output, tt.expected)
		})
	}
}

func TestRenderer_TreeStructure(t *testing.T) {
	tests := []struct {
		name      string
		buildTree func() *StatusTree
		contains  []string
	}{
		{
			name: "single_node",
			buildTree: func() *StatusTree {
				return NewStatusTree("root")
			},
			contains: []string{"root"},
		},
		{
			name: "parent_with_children",
			buildTree: func() *StatusTree {
				tree := NewStatusTree("parent")
				tree.Add("child1")
				tree.Add("child2")
				return tree
			},
			contains: []string{"parent", "child1", "child2", "├──", "└──"},
		},
		{
			name: "deep_nesting",
			buildTree: func() *StatusTree {
				tree := NewStatusTree("level1")
				l2 := tree.Add("level2")
				l3 := l2.Add("level3")
				l3.Add("level4")
				return tree
			},
			contains: []string{"level1", "level2", "level3", "level4"},
		},
		{
			name: "workflow_structure",
			buildTree: func() *StatusTree {
				tree := NewStatusTreeWithStatus("my-workflow", StatusRunning)
				g1 := tree.AddWithStatus("group1", StatusSucceeded)
				g1.AddWithStatus("job1", StatusSucceeded)
				g2 := tree.AddWithStatus("group2", StatusRunning)
				g2.Add(RenderDependency("group1", true))
				g2.AddWithStatus("job2", StatusRunning)
				return tree
			},
			contains: []string{
				"my-workflow", "[running]",
				"group1", "[succeeded]",
				"job1",
				"group2",
				"depends on group: group1",
				"job2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer(false)
			tree := tt.buildTree()

			output := r.Render(tree)

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestRenderer_ColorMode(t *testing.T) {
	tree := NewStatusTreeWithStatus("workflow", StatusSucceeded)

	// Without color
	rNoColor := NewRenderer(false)
	outputNoColor := rNoColor.Render(tree)
	assert.Contains(t, outputNoColor, "[succeeded]")

	// With color - output should still contain the text (ANSI codes are transparent to Contains)
	rColor := NewRenderer(true)
	outputColor := rColor.Render(tree)
	assert.NotEmpty(t, outputColor)
}

func TestRenderer_ComplexWorkflow(t *testing.T) {
	r := NewRenderer(false)

	// Build a complex workflow tree
	workflow := NewStatusTreeWithStatus("production-deploy", StatusRunning)

	// Setup phase
	setup := workflow.AddWithStatus("setup", StatusSucceeded)
	setup.AddWithStatus("configure-aws", StatusSucceeded)
	setup.AddWithStatus("validate-config", StatusSucceeded)

	// Build phase
	build := workflow.AddWithStatus("build", StatusSucceeded)
	build.Add(RenderDependency("setup", true))
	build.AddWithStatus("compile-frontend", StatusSucceeded)
	build.AddWithStatus("compile-backend", StatusSucceeded)
	build.AddWithStatus("run-unit-tests", StatusSucceeded)

	// Deploy phase
	deploy := workflow.AddWithStatus("deploy", StatusRunning)
	deploy.Add(RenderDependency("build", true))
	staging := deploy.AddWithStatus("deploy-staging", StatusSucceeded)
	staging.Add(RenderDependency("compile-frontend", false))
	integration := deploy.AddWithStatus("run-integration-tests", StatusRunning)
	integration.Add(RenderDependency("deploy-staging", false))
	prod := deploy.AddWithStatus("deploy-production", StatusPending)
	prod.Add(RenderDependency("run-integration-tests", false))

	// Cleanup phase
	cleanup := workflow.AddWithStatus("cleanup", StatusPending)
	cleanup.Add(RenderDependency("deploy", true))
	cleanup.AddWithStatus("remove-staging", StatusPending)

	output := r.Render(workflow)

	// Verify structure
	expectedPhrases := []string{
		"production-deploy",
		"setup",
		"configure-aws",
		"build",
		"compile-frontend",
		"deploy",
		"deploy-staging",
		"run-integration-tests",
		"deploy-production",
		"cleanup",
		"depends on group: setup",
		"depends on group: build",
		"depends on: deploy-staging",
	}

	for _, phrase := range expectedPhrases {
		assert.Contains(t, output, phrase, "output should contain: %s", phrase)
	}

	// Verify line structure
	lines := strings.Split(output, "\n")
	assert.Greater(t, len(lines), 10, "should have multiple lines")
}

func TestRenderDependency_Matrix(t *testing.T) {
	tests := []struct {
		name     string
		depName  string
		isGroup  bool
		expected string
	}{
		{
			name:     "job_dependency",
			depName:  "init-job",
			isGroup:  false,
			expected: "depends on: init-job",
		},
		{
			name:     "group_dependency",
			depName:  "setup-group",
			isGroup:  true,
			expected: "depends on group: setup-group",
		},
		{
			name:     "long_job_name",
			depName:  "very-long-job-name-with-many-parts",
			isGroup:  false,
			expected: "depends on: very-long-job-name-with-many-parts",
		},
		{
			name:     "empty_name",
			depName:  "",
			isGroup:  false,
			expected: "depends on: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RenderDependency(tt.depName, tt.isGroup)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test that tree rendering handles edge cases
func TestRenderer_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		buildTree func() *StatusTree
		verify    func(*testing.T, string)
	}{
		{
			name: "empty_text_node",
			buildTree: func() *StatusTree {
				return NewStatusTree("")
			},
			verify: func(t *testing.T, output string) {
				assert.NotEmpty(t, output)
			},
		},
		{
			name: "special_characters_in_text",
			buildTree: func() *StatusTree {
				tree := NewStatusTree("workflow-v1.2.3")
				tree.Add("job_with_underscore")
				tree.Add("job.with.dots")
				return tree
			},
			verify: func(t *testing.T, output string) {
				assert.Contains(t, output, "workflow-v1.2.3")
				assert.Contains(t, output, "job_with_underscore")
				assert.Contains(t, output, "job.with.dots")
			},
		},
		{
			name: "many_siblings",
			buildTree: func() *StatusTree {
				tree := NewStatusTree("parent")
				for i := 0; i < 20; i++ {
					tree.Add("child")
				}
				return tree
			},
			verify: func(t *testing.T, output string) {
				// Should have 19 middle items and 1 last item
				assert.Equal(t, 19, strings.Count(output, "├──"))
				assert.Equal(t, 1, strings.Count(output, "└──"))
			},
		},
		{
			name: "unicode_in_text",
			buildTree: func() *StatusTree {
				tree := NewStatusTree("🚀 deployment")
				tree.Add("✅ verified")
				tree.Add("⏳ pending")
				return tree
			},
			verify: func(t *testing.T, output string) {
				assert.Contains(t, output, "🚀")
				assert.Contains(t, output, "✅")
				assert.Contains(t, output, "⏳")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenderer(false)
			tree := tt.buildTree()
			output := r.Render(tree)
			tt.verify(t, output)
		})
	}
}
