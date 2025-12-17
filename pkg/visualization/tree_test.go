package visualization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TreeTestSuite struct {
	suite.Suite
}

func TestTreeSuite(t *testing.T) {
	suite.Run(t, new(TreeTestSuite))
}

func (s *TreeTestSuite) TestNewStatusTree() {
	tree := NewStatusTree("root")

	s.Equal("root", tree.Text())
	s.Equal("", tree.Status())
	s.Empty(tree.Items())
}

func (s *TreeTestSuite) TestNewStatusTreeWithStatus() {
	tree := NewStatusTreeWithStatus("workflow", StatusRunning)

	s.Equal("workflow", tree.Text())
	s.Equal(StatusRunning, tree.Status())
	s.Empty(tree.Items())
}

func (s *TreeTestSuite) TestAdd() {
	root := NewStatusTree("root")
	child := root.Add("child")

	s.Equal("child", child.Text())
	s.Len(root.Items(), 1)
	s.Equal(child, root.Items()[0])
}

func (s *TreeTestSuite) TestAddWithStatus() {
	root := NewStatusTree("root")
	child := root.AddWithStatus("job", StatusSucceeded)

	s.Equal("job", child.Text())
	s.Equal(StatusSucceeded, child.Status())
	s.Len(root.Items(), 1)
}

func (s *TreeTestSuite) TestChaining() {
	root := NewStatusTree("workflow")
	group := root.Add("group1")
	job := group.AddWithStatus("job1", StatusRunning)
	job.Add("depends on: init-job")

	s.Len(root.Items(), 1)
	s.Len(root.Items()[0].Items(), 1)
	s.Len(root.Items()[0].Items()[0].Items(), 1)
}

func (s *TreeTestSuite) TestItems() {
	root := NewStatusTree("root")
	root.Add("child1")
	root.Add("child2")
	root.Add("child3")

	items := root.Items()
	s.Len(items, 3)
	s.Equal("child1", items[0].Text())
	s.Equal("child2", items[1].Text())
	s.Equal("child3", items[2].Text())
}

// ==================== MATRIX TEST: TREE BUILDING ====================

func TestStatusTree_StatusValues(t *testing.T) {
	statuses := []string{
		StatusPending,
		StatusRunning,
		StatusSucceeded,
		StatusFailed,
		StatusAborted,
		StatusUnknown,
	}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			tree := NewStatusTreeWithStatus("node", status)
			assert.Equal(t, status, tree.Status())
		})
	}
}

func TestStatusTree_DeepNesting(t *testing.T) {
	depth := 10
	root := NewStatusTree("level-0")
	current := root

	for i := 1; i < depth; i++ {
		current = current.Add("level-" + string(rune('0'+i)))
	}

	// Traverse back to verify
	node := root
	for i := 0; i < depth-1; i++ {
		assert.Len(t, node.Items(), 1)
		node = node.Items()[0]
	}
	assert.Empty(t, node.Items())
}

func TestStatusTree_MultipleChildren(t *testing.T) {
	root := NewStatusTree("workflow")

	// Add multiple groups
	for i := 0; i < 5; i++ {
		group := root.AddWithStatus("group"+string(rune('0'+i)), StatusPending)
		// Add jobs to each group
		for j := 0; j < 3; j++ {
			group.AddWithStatus("job"+string(rune('0'+j)), StatusPending)
		}
	}

	assert.Len(t, root.Items(), 5)
	for _, group := range root.Items() {
		assert.Len(t, group.Items(), 3)
	}
}

func TestStatusTree_EmptyTree(t *testing.T) {
	tree := NewStatusTree("")
	assert.Equal(t, "", tree.Text())
	assert.Equal(t, "", tree.Status())
	assert.Empty(t, tree.Items())
}

func TestStatusTree_ComplexWorkflow(t *testing.T) {
	// Simulate a real workflow structure
	workflow := NewStatusTreeWithStatus("my-workflow", StatusRunning)

	// First group - succeeded
	group1 := workflow.AddWithStatus("init-group", StatusSucceeded)
	group1.AddWithStatus("setup-database", StatusSucceeded)
	group1.AddWithStatus("setup-cache", StatusSucceeded)

	// Second group - running
	group2 := workflow.AddWithStatus("build-group", StatusRunning)
	group2.Add("depends on group: init-group")
	build := group2.AddWithStatus("build-app", StatusRunning)
	build.Add("depends on: setup-database")
	group2.AddWithStatus("run-tests", StatusPending).Add("depends on: build-app")

	// Third group - pending
	group3 := workflow.AddWithStatus("deploy-group", StatusPending)
	group3.Add("depends on group: build-group")
	group3.AddWithStatus("deploy-staging", StatusPending)
	group3.AddWithStatus("deploy-production", StatusPending).Add("depends on: deploy-staging")

	assert.Len(t, workflow.Items(), 3)
	assert.Equal(t, StatusSucceeded, workflow.Items()[0].Status())
	assert.Equal(t, StatusRunning, workflow.Items()[1].Status())
	assert.Equal(t, StatusPending, workflow.Items()[2].Status())
}
