package controllers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
)

// TreeTestSuite tests the Tree implementation
type TreeTestSuite struct {
	suite.Suite
}

func TestTreeInterfaceSuite(t *testing.T) {
	suite.Run(t, new(TreeTestSuite))
}

func (s *TreeTestSuite) TestNew() {
	tree := New("root")
	s.Equal("root", tree.Text())
	s.Empty(tree.Items())
}

func (s *TreeTestSuite) TestAdd() {
	root := New("root")
	child := root.Add("child")

	s.Equal("child", child.Text())
	s.Len(root.Items(), 1)
}

func (s *TreeTestSuite) TestAddTree() {
	root := New("root")
	subtree := New("subtree")
	subtree.Add("subtree-child")

	root.AddTree(subtree)

	s.Len(root.Items(), 1)
	s.Equal("subtree", root.Items()[0].Text())
}

func (s *TreeTestSuite) TestItems() {
	root := New("root")
	root.Add("child1")
	root.Add("child2")
	root.Add("child3")

	items := root.Items()
	s.Len(items, 3)
}

func (s *TreeTestSuite) TestText() {
	tree := New("test-text")
	s.Equal("test-text", tree.Text())
}

func (s *TreeTestSuite) TestPrint_Simple() {
	tree := New("root")

	output := tree.Print()

	s.Contains(output, "root")
}

func (s *TreeTestSuite) TestPrint_WithChildren() {
	tree := New("root")
	tree.Add("child1")
	tree.Add("child2")

	output := tree.Print()

	s.Contains(output, "root")
	s.Contains(output, "child1")
	s.Contains(output, "child2")
	s.Contains(output, "├──")
	s.Contains(output, "└──")
}

func (s *TreeTestSuite) TestPrint_NestedChildren() {
	tree := New("root")
	child := tree.Add("child")
	child.Add("grandchild1")
	child.Add("grandchild2")

	output := tree.Print()

	s.Contains(output, "root")
	s.Contains(output, "child")
	s.Contains(output, "grandchild1")
	s.Contains(output, "grandchild2")
}

func (s *TreeTestSuite) TestPrint_WorkflowStructure() {
	workflow := New("my-workflow")
	group1 := workflow.Add("init-group")
	group1.Add("setup-job")
	group1.Add("config-job")

	group2 := workflow.Add("build-group")
	group2.Add("Depends on group: init-group")
	group2.Add("compile-job")

	output := workflow.Print()

	s.Contains(output, "my-workflow")
	s.Contains(output, "init-group")
	s.Contains(output, "setup-job")
	s.Contains(output, "build-group")
	s.Contains(output, "Depends on group: init-group")
}

// ==================== GENERATE DEPENDENCY TREE TESTS ====================

type DependencyTreeTestSuite struct {
	suite.Suite
	ctx        context.Context
	cancel     context.CancelFunc
	client     *MockClient
	reconciler *ManagedJobReconciler
}

func (s *DependencyTreeTestSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.client = NewMockClient()
	s.reconciler = &ManagedJobReconciler{
		Client:   s.client,
		Scheme:   s.client.Scheme(),
		Recorder: NewFakeRecorder(),
	}
}

func (s *DependencyTreeTestSuite) TearDownTest() {
	s.cancel()
}

func TestDependencyTreeSuite(t *testing.T) {
	suite.Run(t, new(DependencyTreeTestSuite))
}

func (s *DependencyTreeTestSuite) newConnPackage(mj *jobsmanagerv1beta1.ManagedJob) *connPackage {
	cp := &connPackage{
		ctx: s.ctx,
		r:   s.reconciler,
		mj:  mj,
		req: ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      mj.Name,
				Namespace: mj.Namespace,
			},
		},
		logger: zap.New(),
	}
	cp.buildDependencyMaps()
	return cp
}

func (s *DependencyTreeTestSuite) TestGenerateDependencyTree_SingleGroup() {
	mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox"),
			NewTestJobDef("job2", "busybox"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)

	cp := s.newConnPackage(mj)
	cp.generateDependencyTree()

	// Sequential jobs should have dependencies
	s.NotEmpty(mj.Spec.Groups[0].Jobs[1].Dependencies)
}

func (s *DependencyTreeTestSuite) TestGenerateDependencyTree_ParallelJobs() {
	mj := &jobsmanagerv1beta1.ManagedJob{
		ObjectMeta: NewTestManagedJob("workflow", "default", nil).ObjectMeta,
		Spec: jobsmanagerv1beta1.ManagedJobSpec{
			Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
				{
					Name: "group1",
					Jobs: []*jobsmanagerv1beta1.ManagedJobDefinition{
						{Name: "job1", Image: "busybox", Parallel: true, Status: ExecutionStatusPending},
						{Name: "job2", Image: "busybox", Parallel: true, Status: ExecutionStatusPending},
					},
					Status: ExecutionStatusPending,
				},
			},
		},
	}
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)

	cp := s.newConnPackage(mj)
	cp.generateDependencyTree()

	// Parallel jobs should not have dependencies on each other
	s.Empty(mj.Spec.Groups[0].Jobs[0].Dependencies)
	s.Empty(mj.Spec.Groups[0].Jobs[1].Dependencies)
}

func (s *DependencyTreeTestSuite) TestGenerateDependencyTree_MultipleGroups() {
	mj := NewTestManagedJob("workflow", "default", []*jobsmanagerv1beta1.ManagedJobGroup{
		NewTestGroup("group1", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job1", "busybox"),
		}),
		NewTestGroup("group2", []*jobsmanagerv1beta1.ManagedJobDefinition{
			NewTestJobDef("job2", "busybox"),
		}),
	})
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)

	cp := s.newConnPackage(mj)
	cp.generateDependencyTree()

	// Sequential groups should have dependencies
	s.NotEmpty(mj.Spec.Groups[1].Dependencies)
}

func (s *DependencyTreeTestSuite) TestGenerateDependencyTree_ParallelGroups() {
	mj := &jobsmanagerv1beta1.ManagedJob{
		ObjectMeta: NewTestManagedJob("workflow", "default", nil).ObjectMeta,
		Spec: jobsmanagerv1beta1.ManagedJobSpec{
			Groups: []*jobsmanagerv1beta1.ManagedJobGroup{
				{
					Name:     "group1",
					Jobs:     []*jobsmanagerv1beta1.ManagedJobDefinition{{Name: "job1", Image: "busybox", Status: ExecutionStatusPending}},
					Parallel: true,
					Status:   ExecutionStatusPending,
				},
				{
					Name:     "group2",
					Jobs:     []*jobsmanagerv1beta1.ManagedJobDefinition{{Name: "job2", Image: "busybox", Status: ExecutionStatusPending}},
					Parallel: true,
					Status:   ExecutionStatusPending,
				},
			},
		},
	}
	controllerutil.AddFinalizer(mj, FinalizerName)
	s.client.AddManagedJob(mj)

	cp := s.newConnPackage(mj)
	cp.generateDependencyTree()

	// Parallel groups should not have dependencies on each other
	s.Empty(mj.Spec.Groups[0].Dependencies)
	s.Empty(mj.Spec.Groups[1].Dependencies)
}

func (s *DependencyTreeTestSuite) TestCheckIfPresentInDependencies_Found() {
	mj := NewTestManagedJob("workflow", "default", nil)
	cp := s.newConnPackage(mj)

	deps := []*jobsmanagerv1beta1.ManagedJobDependencies{
		{Name: "dep1", Status: ExecutionStatusPending},
		{Name: "dep2", Status: ExecutionStatusPending},
	}

	s.True(cp.checkIfPresentInDependencies(deps, "dep1"))
	s.True(cp.checkIfPresentInDependencies(deps, "dep2"))
}

func (s *DependencyTreeTestSuite) TestCheckIfPresentInDependencies_NotFound() {
	mj := NewTestManagedJob("workflow", "default", nil)
	cp := s.newConnPackage(mj)

	deps := []*jobsmanagerv1beta1.ManagedJobDependencies{
		{Name: "dep1", Status: ExecutionStatusPending},
	}

	s.False(cp.checkIfPresentInDependencies(deps, "dep3"))
}

func (s *DependencyTreeTestSuite) TestCheckIfPresentInDependencies_Empty() {
	mj := NewTestManagedJob("workflow", "default", nil)
	cp := s.newConnPackage(mj)

	var deps []*jobsmanagerv1beta1.ManagedJobDependencies

	s.False(cp.checkIfPresentInDependencies(deps, "any"))
}

// ==================== MATRIX TEST: TREE PRINTING ====================

func TestTreePrint_Matrix(t *testing.T) {
	tests := []struct {
		name      string
		buildTree func() Tree
		contains  []string
	}{
		{
			name: "single_node",
			buildTree: func() Tree {
				return New("root")
			},
			contains: []string{"root"},
		},
		{
			name: "two_children",
			buildTree: func() Tree {
				tree := New("parent")
				tree.Add("child1")
				tree.Add("child2")
				return tree
			},
			contains: []string{"parent", "├──", "└──", "child1", "child2"},
		},
		{
			name: "deep_nesting",
			buildTree: func() Tree {
				tree := New("l1")
				l2 := tree.Add("l2")
				l3 := l2.Add("l3")
				l3.Add("l4")
				return tree
			},
			contains: []string{"l1", "l2", "l3", "l4"},
		},
		{
			name: "workflow_example",
			buildTree: func() Tree {
				wf := New("my-workflow")
				g1 := wf.Add("init")
				g1.Add("setup-db")
				g1.Add("setup-cache")
				g2 := wf.Add("build")
				g2.Add("Depends on group: init")
				g2.Add("compile")
				return wf
			},
			contains: []string{"my-workflow", "init", "setup-db", "build", "Depends on group: init"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := tt.buildTree()
			output := tree.Print()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestTreePrint_MultilineText(t *testing.T) {
	tree := New("root")
	tree.Add("line1\nline2\nline3")

	output := tree.Print()

	// Should have all three lines
	assert.True(t, strings.Contains(output, "line1"))
	assert.True(t, strings.Contains(output, "line2"))
	assert.True(t, strings.Contains(output, "line3"))
}
