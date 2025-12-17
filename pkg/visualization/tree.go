package visualization

// StatusTree represents a tree node with text and execution status
type StatusTree struct {
	text   string
	status string
	items  []*StatusTree
}

// NewStatusTree creates a new StatusTree node
func NewStatusTree(text string) *StatusTree {
	return &StatusTree{
		text:  text,
		items: make([]*StatusTree, 0),
	}
}

// NewStatusTreeWithStatus creates a new StatusTree node with a status
func NewStatusTreeWithStatus(text, status string) *StatusTree {
	return &StatusTree{
		text:   text,
		status: status,
		items:  make([]*StatusTree, 0),
	}
}

// Add creates and appends a child node, returning it for chaining
func (t *StatusTree) Add(text string) *StatusTree {
	child := NewStatusTree(text)
	t.items = append(t.items, child)
	return child
}

// AddWithStatus creates and appends a child node with status
func (t *StatusTree) AddWithStatus(text, status string) *StatusTree {
	child := NewStatusTreeWithStatus(text, status)
	t.items = append(t.items, child)
	return child
}

// Items returns all child nodes
func (t *StatusTree) Items() []*StatusTree {
	return t.items
}

// Text returns the node's text value
func (t *StatusTree) Text() string {
	return t.text
}

// Status returns the node's status value
func (t *StatusTree) Status() string {
	return t.status
}
