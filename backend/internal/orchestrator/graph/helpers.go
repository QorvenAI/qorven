// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package graph

import (
	"encoding/json"

	"github.com/qorvenai/qorven/internal/plans"
)

// artifactsJSON marshals an arbitrary artifacts blob into the JSONB-
// compatible shape the plans store expects. Nil → nil (store treats as
// no-op merge).
func artifactsJSON(in any) json.RawMessage {
	if in == nil {
		return nil
	}
	if raw, ok := in.(json.RawMessage); ok {
		return raw
	}
	if m, ok := in.(map[string]any); ok && len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	return b
}

// allTerminal reports whether every node is in a terminal state.
func allTerminal(nodes []*plans.Node) bool {
	for _, n := range nodes {
		switch n.State {
		case plans.NodeDone, plans.NodeFailed, plans.NodeCancelled:
			continue
		default:
			return false
		}
	}
	return true
}

// anyFailed reports whether any node is in failed state.
func anyFailed(nodes []*plans.Node) bool {
	for _, n := range nodes {
		if n.State == plans.NodeFailed {
			return true
		}
	}
	return false
}

// artifactHasString checks whether the JSONB artifacts map has the given
// key set to the expected string. Tolerant of malformed payloads.
func artifactHasString(raw json.RawMessage, key, want string) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, ok := m[key].(string)
	return ok && v == want
}
