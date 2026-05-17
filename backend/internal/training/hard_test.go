// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package training

import "testing"

func TestHard_Training_ExampleFormats(t *testing.T) {
	example := TrainingExample{
		Messages: []TrainingMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
		},
	}
	if len(example.Messages) != 3 { t.Error("messages") }
	if example.Messages[0].Role != "system" { t.Error("first should be system") }
	if example.Messages[2].Role != "assistant" { t.Error("last should be assistant") }
}

func TestHard_Training_PreferencePairs(t *testing.T) {
	pairs := []PreferencePair{
		{Prompt: "Explain AI", Chosen: "AI is artificial intelligence, a field of computer science...", Rejected: "idk"},
		{Prompt: "Write code", Chosen: "```go\nfunc main() {}\n```", Rejected: "no"},
	}
	for _, p := range pairs {
		if len(p.Chosen) <= len(p.Rejected) { t.Logf("chosen shorter than rejected for %q", p.Prompt) }
	}
}

func TestHard_Training_FeedbackTypes(t *testing.T) {
	types := []string{"like", "dislike", "superlike", "correction"}
	for _, ft := range types {
		r := FeedbackRecord{FeedbackType: ft}
		if r.FeedbackType != ft { t.Errorf("type=%q", r.FeedbackType) }
	}
}
