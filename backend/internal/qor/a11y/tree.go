// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package a11y

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/chromedp"
)

// tree.go — Fetch and parse the accessibility tree from a browser page.
// The accessibility tree is the primary way AI agents "see" web pages.

// Node represents a node in the parsed accessibility tree.
type Node struct {
	ID          string            `json:"id"`
	Role        string            `json:"role"`
	Name        string            `json:"name"`
	Value       string            `json:"value,omitempty"`
	Description string            `json:"description,omitempty"`
	Focused     bool              `json:"focused,omitempty"`
	Disabled    bool              `json:"disabled,omitempty"`
	Checked     string            `json:"checked,omitempty"`
	Expanded    string            `json:"expanded,omitempty"`
	Level       int               `json:"level,omitempty"`
	BackendID   int64             `json:"backend_id,omitempty"`
	FrameID     string            `json:"frame_id,omitempty"`
	Children    []*Node           `json:"children,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
}

// DOMState holds the full accessibility DOM snapshot.
type DOMState struct {
	Root       *Node            `json:"root"`
	NodeCount  int              `json:"node_count"`
	FrameCount int              `json:"frame_count"`
	Elapsed    time.Duration    `json:"elapsed"`
	NodeMap    map[string]*Node `json:"-"`
}

// FetchTree retrieves the full accessibility tree from the current page.
func FetchTree(ctx context.Context) (*DOMState, error) {
	start := time.Now()

	var rawNodes []*accessibility.Node
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		nodes, err := accessibility.GetFullAXTree().Do(ctx)
		if err != nil { return err }
		rawNodes = nodes
		return nil
	})); err != nil {
		return nil, fmt.Errorf("a11y.fetch: %w", err)
	}

	// Parse into our Node type
	nodeMap := make(map[string]*Node, len(rawNodes))
	children := make(map[string][]string)
	var rootID string

	for _, raw := range rawNodes {
		id := string(raw.NodeID)
		node := parseNode(raw)
		nodeMap[id] = node

		if raw.ParentID != "" {
			children[string(raw.ParentID)] = append(children[string(raw.ParentID)], id)
		} else if rootID == "" {
			rootID = id
		}
	}

	// Link children
	for parentID, childIDs := range children {
		parent, ok := nodeMap[parentID]
		if !ok { continue }
		for _, cid := range childIDs {
			if child, ok := nodeMap[cid]; ok {
				parent.Children = append(parent.Children, child)
			}
		}
	}

	root := nodeMap[rootID]
	state := &DOMState{
		Root:      root,
		NodeCount: len(nodeMap),
		Elapsed:   time.Since(start),
		NodeMap:   nodeMap,
	}

	// Count unique frames
	frames := map[string]bool{}
	for _, raw := range rawNodes {
		if raw.FrameID != "" { frames[string(raw.FrameID)] = true }
	}
	state.FrameCount = len(frames)

	slog.Debug("a11y.fetch", "nodes", state.NodeCount, "frames", state.FrameCount, "elapsed", state.Elapsed)
	return state, nil
}

func parseNode(raw *accessibility.Node) *Node {
	n := &Node{
		ID:        string(raw.NodeID),
		Role:      valStr(raw.Role),
		Name:      valStr(raw.Name),
		Value:     valStr(raw.Value),
		Description: valStr(raw.Description),
		BackendID: int64(raw.BackendDOMNodeID),
		FrameID:   string(raw.FrameID),
	}

	// Parse properties
	for _, p := range raw.Properties {
		pname := string(p.Name)
		pval := valStr(p.Value)
		switch pname {
		case "focused":  n.Focused = pval == "true"
		case "disabled": n.Disabled = pval == "true"
		case "checked":  n.Checked = pval
		case "expanded": n.Expanded = pval
		case "level":
			if n.Properties == nil { n.Properties = map[string]string{} }
			n.Properties["level"] = pval
		}
	}
	return n
}

func valStr(v *accessibility.Value) string {
	if v == nil { return "" }
	raw := []byte(v.Value)
	if len(raw) == 0 { return "" }
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil { return s }
	}
	return string(raw)
}
