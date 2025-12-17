package visualization

import (
	"strings"

	"github.com/fatih/color"
)

// Box-drawing characters for tree rendering
const (
	newLine      = "\n"
	emptySpace   = "    "
	middleItem   = "\u251c\u2500\u2500 " // ├──
	continueItem = "\u2502   "           // │
	lastItem     = "\u2514\u2500\u2500 " // └──
)

// Status constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusAborted   = "aborted"
	StatusUnknown   = "unknown"
)

// Renderer handles tree rendering with optional color support
type Renderer struct {
	useColor bool
	green    *color.Color
	yellow   *color.Color
	red      *color.Color
	gray     *color.Color
	magenta  *color.Color
	cyan     *color.Color
}

// NewRenderer creates a new Renderer
func NewRenderer(useColor bool) *Renderer {
	return &Renderer{
		useColor: useColor,
		green:    color.New(color.FgGreen),
		yellow:   color.New(color.FgYellow),
		red:      color.New(color.FgRed),
		gray:     color.New(color.FgHiBlack),
		magenta:  color.New(color.FgMagenta),
		cyan:     color.New(color.FgCyan),
	}
}

// Render renders a StatusTree to a string
func (r *Renderer) Render(t *StatusTree) string {
	var sb strings.Builder
	r.renderNode(&sb, t, []bool{})
	return sb.String()
}

// renderNode renders a single node and its children
func (r *Renderer) renderNode(sb *strings.Builder, t *StatusTree, spaces []bool) {
	// Render current node
	r.renderText(sb, t.Text(), t.Status(), spaces, true)

	// Render children
	items := t.Items()
	for i, child := range items {
		isLast := i == len(items)-1
		r.renderChild(sb, child, spaces, isLast)
	}
}

// renderChild renders a child node with proper indentation
func (r *Renderer) renderChild(sb *strings.Builder, t *StatusTree, spaces []bool, isLast bool) {
	// Add prefix based on whether this is the last item
	for _, space := range spaces {
		if space {
			sb.WriteString(emptySpace)
		} else {
			sb.WriteString(continueItem)
		}
	}

	if isLast {
		sb.WriteString(lastItem)
	} else {
		sb.WriteString(middleItem)
	}

	// Render the text with status
	r.renderTextInline(sb, t.Text(), t.Status())
	sb.WriteString(newLine)

	// Render children with updated spaces
	newSpaces := append(spaces, isLast)
	items := t.Items()
	for i, child := range items {
		childIsLast := i == len(items)-1
		r.renderChild(sb, child, newSpaces, childIsLast)
	}
}

// renderText renders the root node text
func (r *Renderer) renderText(sb *strings.Builder, text, status string, spaces []bool, isRoot bool) {
	if isRoot {
		r.renderTextInline(sb, text, status)
		sb.WriteString(newLine)
	}
}

// renderTextInline renders text with status inline
func (r *Renderer) renderTextInline(sb *strings.Builder, text, status string) {
	sb.WriteString(text)
	if status != "" {
		sb.WriteString(" ")
		r.renderStatus(sb, status)
	}
}

// renderStatus renders the status with appropriate color
func (r *Renderer) renderStatus(sb *strings.Builder, status string) {
	statusText := "[" + status + "]"

	if !r.useColor {
		sb.WriteString(statusText)
		return
	}

	switch status {
	case StatusSucceeded:
		sb.WriteString(r.green.Sprint(statusText))
	case StatusRunning:
		sb.WriteString(r.yellow.Sprint(statusText))
	case StatusFailed:
		sb.WriteString(r.red.Sprint(statusText))
	case StatusPending:
		sb.WriteString(r.gray.Sprint(statusText))
	case StatusAborted:
		sb.WriteString(r.magenta.Sprint(statusText))
	default:
		sb.WriteString(r.cyan.Sprint(statusText))
	}
}

// RenderDependency formats a dependency reference
func RenderDependency(name string, isGroup bool) string {
	if isGroup {
		return "depends on group: " + name
	}
	return "depends on: " + name
}
