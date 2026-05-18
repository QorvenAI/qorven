// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package scenario

import "testing"

func TestAgent_Fields(t *testing.T) {
	a := Agent{Name: "Alice", Role: "CEO", Bio: "Strategic thinker"}
	if a.Name != "Alice" { t.Error("wrong name") }
}

func TestRound_Fields(t *testing.T) {
	r := Round{Number: 1, AgentName: "Alice", Content: "I think we should..."}
	if r.Number != 1 { t.Error("wrong number") }
}

func TestPickSpeakers(t *testing.T) {
	agents := []Agent{{Name: "A"}, {Name: "B"}, {Name: "C"}}
	speakers := pickSpeakers(agents, 2)
	if len(speakers) != 2 { t.Errorf("expected 2, got %d", len(speakers)) }
}

func TestFormatAgents(t *testing.T) {
	agents := []Agent{{Name: "Alice", Role: "CEO"}}
	result := formatAgents(agents)
	if result == "" { t.Error("empty") }
}

func TestEngine_New(t *testing.T) {
	e := NewEngine(nil)
	if e == nil { t.Fatal("nil") }
}
