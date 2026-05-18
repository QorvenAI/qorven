// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tracing

import "testing"

func TestStore_New(t *testing.T) {
	s := NewStore(nil)
	if s == nil { t.Fatal("nil") }
}

func TestTrace_Fields(t *testing.T) {
	tr := Trace{ID: "t1", AgentID: "a1", Model: "gpt-4", Status: "completed"}
	if tr.Model != "gpt-4" { t.Error("wrong model") }
}

func TestSpan_Fields(t *testing.T) {
	sp := Span{ID: "s1", TraceID: "t1", Name: "llm_call", Kind: "llm"}
	if sp.Kind != "llm" { t.Error("wrong kind") }
}

func TestSnapshotWorker_New(t *testing.T) {
	w := NewSnapshotWorker(nil, nil)
	if w == nil { t.Fatal("nil") }
}

func TestSnapshotWorker_Stop_NotStarted(t *testing.T) {
	w := NewSnapshotWorker(nil, nil)
	w.Stop() // should not panic
}
