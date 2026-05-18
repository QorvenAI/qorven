// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/chromedp"
)

// snapshot.go — Capture page accessibility tree as text for LLM consumption.
// This is the key feature: converts a visual webpage into a text representation
// that an AI agent can understand and act on.

// TakeSnapshot captures the accessibility tree and formats it for LLM input.
func (m *Manager) TakeSnapshot(ctx context.Context) (*SnapshotResult, error) {
	m.mu.Lock()
	if !m.running { m.mu.Unlock(); return nil, fmt.Errorf("browser not running") }
	bctx := m.ctx
	m.mu.Unlock()

	tctx, cancel := context.WithTimeout(bctx, m.cfg.ActionTimeout)
	defer cancel()

	// Get page URL and title
	var url, title string
	chromedp.Run(tctx, chromedp.Location(&url), chromedp.Title(&title))

	// Fetch full accessibility tree via CDP
	var nodes []*accessibility.Node
	if err := chromedp.Run(tctx, chromedp.ActionFunc(func(ctx context.Context) error {
		snapshot, err := accessibility.GetFullAXTree().Do(ctx)
		if err != nil { return err }
		nodes = snapshot
		return nil
	})); err != nil {
		return nil, fmt.Errorf("browser.snapshot: %w", err)
	}

	// Build text tree and refs
	tree, refs, stats := formatAXTree(nodes)

	return &SnapshotResult{
		URL:   url,
		Title: title,
		Tree:  tree,
		Stats: stats,
		Refs:  refs,
	}, nil
}

// formatAXTree converts accessibility nodes into a text tree with reference IDs.
func formatAXTree(nodes []*accessibility.Node) (string, map[string]Ref, SnapshotStats) {
	refs := make(map[string]Ref)
	var sb strings.Builder
	maxDepth := 0
	nodeCount := 0
	refCounter := 0

	// Build parent→children map
	children := map[string][]string{}
	nodeMap := map[string]*accessibility.Node{}
	var rootID string

	for _, n := range nodes {
		id := string(n.NodeID)
		nodeMap[id] = n
		if n.ParentID != "" {
			children[string(n.ParentID)] = append(children[string(n.ParentID)], id)
		} else if rootID == "" {
			rootID = id
		}
	}

	// DFS traversal
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		n, ok := nodeMap[id]
		if !ok { return }

		role := axStr(n.Role)
		name := axStr(n.Name)

		// Skip ignored/generic nodes
		if role == "none" || role == "generic" || role == "InlineTextBox" {
			for _, childID := range children[id] { walk(childID, depth) }
			return
		}

		nodeCount++
		if depth > maxDepth { maxDepth = depth }

		indent := strings.Repeat("  ", depth)
		sb.WriteString(indent)

		// Assign ref for interactive elements
		if isInteractive(role) {
			refCounter++
			refID := fmt.Sprintf("e%d", refCounter)
			sb.WriteString("[" + refID + "] ")
			refs[refID] = Ref{Role: role, Name: name, NodeID: string(n.NodeID), BackendID: int64(n.BackendDOMNodeID)}
		}

		sb.WriteString(role)
		if name != "" { sb.WriteString(` "` + truncate(name, 80) + `"`) }

		// Show value for inputs
		if v := axStr(n.Value); v != "" { sb.WriteString(" value=" + truncate(v, 40)) }

		// Show checked/disabled state
		for _, p := range n.Properties {
			switch string(p.Name) {
			case "disabled":
				if p.Value != nil && fmt.Sprint(p.Value.Value) == "true" { sb.WriteString(" [disabled]") }
			case "checked":
				if p.Value != nil && fmt.Sprint(p.Value.Value) == "true" { sb.WriteString(" [checked]") }
			case "expanded":
				if p.Value != nil { sb.WriteString(" [expanded=" + fmt.Sprint(p.Value.Value) + "]") }
			}
		}

		sb.WriteString("\n")

		for _, childID := range children[id] { walk(childID, depth+1) }
	}

	if rootID != "" { walk(rootID, 0) }

	return sb.String(), refs, SnapshotStats{Nodes: nodeCount, MaxDepth: maxDepth}
}

func axStr(v *accessibility.Value) string {
	if v == nil { return "" }
	raw := []byte(v.Value)
	if len(raw) == 0 { return "" }
	// jsontext.Value is raw JSON — unquote if string
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil { return s }
	}
	return string(raw)
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

func truncate(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "…"
}
