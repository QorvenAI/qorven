// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package a11y

import (
	"fmt"
	"strings"
)

// serialize.go — Convert accessibility tree to text formats for LLM consumption.

// Serialize converts the tree to a compact text representation.
// Interactive elements get reference IDs (e.g., [e1], [e2]) for agent actions.
func Serialize(root *Node) (string, map[string]NodeRef) {
	if root == nil { return "", nil }
	refs := make(map[string]NodeRef)
	counter := 0
	text := serializeNode(root, 0, refs, &counter)
	return text, refs
}

// NodeRef maps a reference ID to a DOM element for agent actions.
type NodeRef struct {
	Role      string `json:"role"`
	Name      string `json:"name"`
	NodeID    string `json:"node_id"`
	BackendID int64  `json:"backend_id"`
}

func serializeNode(n *Node, depth int, refs map[string]NodeRef, counter *int) string {
	if n == nil { return "" }

	// Skip ignored/generic nodes — pass through to children
	if n.Role == "none" || n.Role == "generic" || n.Role == "InlineTextBox" || n.Role == "" {
		var sb strings.Builder
		for _, child := range n.Children {
			sb.WriteString(serializeNode(child, depth, refs, counter))
		}
		return sb.String()
	}

	var sb strings.Builder
	indent := strings.Repeat("  ", depth)
	sb.WriteString(indent)

	// Assign ref for interactive elements
	if isInteractive(n.Role) {
		*counter++
		refID := fmt.Sprintf("e%d", *counter)
		sb.WriteString("[" + refID + "] ")
		refs[refID] = NodeRef{Role: n.Role, Name: n.Name, NodeID: n.ID, BackendID: n.BackendID}
	}

	sb.WriteString(n.Role)
	if n.Name != "" { sb.WriteString(` "` + truncStr(n.Name, 80) + `"`) }
	if n.Value != "" { sb.WriteString(" value=" + truncStr(n.Value, 40)) }
	if n.Focused { sb.WriteString(" [focused]") }
	if n.Disabled { sb.WriteString(" [disabled]") }
	if n.Checked == "true" { sb.WriteString(" [checked]") }
	if n.Expanded != "" { sb.WriteString(" [expanded=" + n.Expanded + "]") }
	sb.WriteString("\n")

	for _, child := range n.Children {
		sb.WriteString(serializeNode(child, depth+1, refs, counter))
	}
	return sb.String()
}

// SerializeCompact produces a minimal representation (role + name only, no indentation).
func SerializeCompact(root *Node) string {
	if root == nil { return "" }
	var parts []string
	collectCompact(root, &parts)
	return strings.Join(parts, " | ")
}

func collectCompact(n *Node, parts *[]string) {
	if n == nil { return }
	if n.Role != "none" && n.Role != "generic" && n.Role != "" && n.Name != "" {
		*parts = append(*parts, n.Role+":"+truncStr(n.Name, 40))
	}
	for _, child := range n.Children { collectCompact(child, parts) }
}

// CountInteractive returns the number of interactive elements in the tree.
func CountInteractive(root *Node) int {
	if root == nil { return 0 }
	count := 0
	if isInteractive(root.Role) { count++ }
	for _, child := range root.Children { count += CountInteractive(child) }
	return count
}

func isInteractive(role string) bool {
	switch role {
	case "button", "link", "textbox", "checkbox", "radio", "combobox",
		"menuitem", "tab", "switch", "slider", "spinbutton", "searchbox",
		"option", "menuitemcheckbox", "menuitemradio", "treeitem":
		return true
	}
	return false
}

func truncStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "…"
}
