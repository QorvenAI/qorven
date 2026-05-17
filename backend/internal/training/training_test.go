// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package training

import "testing"

func TestFeedbackRecord_Fields(t *testing.T) {
	r := FeedbackRecord{AgentID: "a1", SessionID: "s1", FeedbackType: "like"}
	if r.FeedbackType != "like" { t.Error("wrong rating") }
}

func TestTrainingExample_Fields(t *testing.T) {
	e := TrainingExample{
		Messages: []TrainingMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
		},
	}
	if len(e.Messages) != 3 { t.Error("wrong message count") }
	if e.Messages[0].Role != "system" { t.Error("wrong role") }
}

func TestPreferencePair_Fields(t *testing.T) {
	p := PreferencePair{
		Prompt:   "What is AI?",
		Chosen:   "AI is artificial intelligence...",
		Rejected: "I don't know",
	}
	if p.Prompt == "" { t.Error("empty prompt") }
	if len(p.Chosen) <= len(p.Rejected) { t.Log("chosen should typically be longer") }
}

func TestExporter_New(t *testing.T) {
	e := NewExporter(nil)
	if e == nil { t.Fatal("nil exporter") }
}

func TestTrainingMessage_Roles(t *testing.T) {
	roles := []string{"system", "user", "assistant", "tool"}
	for _, role := range roles {
		m := TrainingMessage{Role: role, Content: "test"}
		if m.Role != role { t.Errorf("wrong role: %q", m.Role) }
	}
}
