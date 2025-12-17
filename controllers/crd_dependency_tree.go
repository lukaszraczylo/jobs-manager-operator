package controllers

import (
	"strings"

	"github.com/lukaszraczylo/pandati"
	jobsmanagerv1beta1 "raczylo.com/jobs-manager-operator/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	newLine      = "\n"
	emptySpace   = "    "
	middleItem   = "├── "
	continueItem = "│   "
	lastItem     = "└── "
)

func New(text string) Tree {
	return &tree{
		text:  text,
		items: []Tree{},
	}
}

// Add adds a node to the tree
func (t *tree) Add(text string) Tree {
	n := New(text)
	t.items = append(t.items, n)
	return n
}

// AddTree adds a tree as an item
func (t *tree) AddTree(tree Tree) {
	t.items = append(t.items, tree)
}

// Text returns the node's value
func (t *tree) Text() string {
	return t.text
}

// Items returns all items in the tree
func (t *tree) Items() []Tree {
	return t.items
}

func (t *tree) Print() string {
	p := &printer{}
	return p.Print(t)
}

func (p *printer) Print(t Tree) string {
	return t.Text() + newLine + p.printItems(t.Items(), []bool{})
}

func (p *printer) printText(text string, spaces []bool, last bool) string {
	var result string
	for _, space := range spaces {
		if space {
			result += emptySpace
		} else {
			result += continueItem
		}
	}

	indicator := middleItem
	if last {
		indicator = lastItem
	}

	var out string
	lines := strings.Split(text, "\n")
	for i := range lines {
		text := lines[i]
		if i == 0 {
			out += result + indicator + text + newLine
			continue
		}
		if last {
			indicator = emptySpace
		} else {
			indicator = continueItem
		}
		out += result + indicator + text + newLine
	}

	return out
}

func (p *printer) printItems(t []Tree, spaces []bool) string {
	var result string
	for i, f := range t {
		last := i == len(t)-1
		result += p.printText(f.Text(), spaces, last)
		if len(f.Items()) > 0 {
			spacesChild := append(spaces, last)
			result += p.printItems(f.Items(), spacesChild)
		}
	}
	return result
}

func (cp *connPackage) checkIfPresentInDependencies(currentDependencies []*jobsmanagerv1beta1.ManagedJobDependencies, dependencyName string) bool {
	for _, dependency := range currentDependencies {
		if dependency.Name == dependencyName {
			return true
		}
	}
	return false
}

func (cp *connPackage) generateDependencyTree() {
	// First pass - initialize the tree and get all the gathered jobs
	originalMainJobDefinition := cp.mj.DeepCopy()

	mainTree := New(cp.mj.Name)
	for _, group := range cp.mj.Spec.Groups {
		groupTree := mainTree.Add(group.Name)
		for _, job := range group.Jobs {
			jobTree := groupTree.Add(job.Name)
			job.CompiledParams = cp.compileParameters(cp.mj.Spec.Params, group.Params, job.Params)
			if job.Parallel {
				continue
			} else {
				// get the groupTree items before this job and add them as dependencies
				for _, jobTreePrevious := range groupTree.Items() {
					if jobTreePrevious.Text() == job.Name {
						break
					}
					generatedJobName := jobNameGenerator(cp.mj.Name, group.Name, jobTreePrevious.Text())
					jobTree.Add("Depends on: " + generatedJobName)
					if !cp.checkIfPresentInDependencies(job.Dependencies, generatedJobName) {
						job.Dependencies = append(job.Dependencies, &jobsmanagerv1beta1.ManagedJobDependencies{Name: generatedJobName, Status: ExecutionStatusPending})
					}
				}
			}
		}
		if group.Parallel {
			continue
		} else {
			// get the mainTree items before this group and add them as dependencies
			for _, groupTreePrevious := range mainTree.Items() {
				if groupTreePrevious.Text() == group.Name {
					break
				}
				groupTree.Add("Depends on group: " + groupTreePrevious.Text())
				if !cp.checkIfPresentInDependencies(group.Dependencies, groupTreePrevious.Text()) {
					group.Dependencies = append(group.Dependencies, &jobsmanagerv1beta1.ManagedJobDependencies{Name: groupTreePrevious.Text(), Status: ExecutionStatusPending})
				}
			}
		}
	}

	_, theSame, _ := pandati.CompareStructsReplaced(originalMainJobDefinition, cp.mj)
	if !theSame {
		if err := cp.updateCRDStatusDirectly(); err != nil {
			log.Log.Error(err, "Failed to update CRD status in dependency tree")
		}
	}
	// fmt.Print(mainTree.Print())
	// fmt.Printf("Dependency tree: %# v", pretty.Formatter(mainTree))
}
