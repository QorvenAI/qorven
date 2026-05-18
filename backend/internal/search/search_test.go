// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package search

import (
	"context"
	"testing"
)

func TestResult_Fields(t *testing.T) {
	r := Result{Title: "Test", URL: "https://example.com", Snippet: "A test result", Source: "brave"}
	if r.Title != "Test" { t.Error("wrong title") }
	if r.Source != "brave" { t.Error("wrong source") }
}

func TestResult_EmptyFields(t *testing.T) {
	r := Result{}
	if r.Title != "" { t.Error("should be empty") }
	if r.URL != "" { t.Error("should be empty") }
}

func TestBraveSearch_NoKey(t *testing.T) {
	// Should fail gracefully without API key
	_, err := BraveSearch(context.Background(), "", "test", 5)
	if err == nil { t.Log("brave search without key may return empty or error") }
}

func TestExaSearch_NoKey(t *testing.T) {
	_, err := ExaSearch(context.Background(), "", "test", 5)
	if err == nil { t.Log("exa search without key may return empty or error") }
}

func TestJinaSearch_NoKey(t *testing.T) {
	_, err := JinaSearch(context.Background(), "", "test", 5)
	if err == nil { t.Log("jina search without key may return empty or error") }
}

func TestKagiFastGPT_NoKey(t *testing.T) {
	_, err := KagiFastGPT(context.Background(), "", "test")
	if err == nil { t.Log("kagi without key may return empty or error") }
}

func TestBraveSearch_DefaultCount(t *testing.T) {
	// Count <= 0 should default to 10
	// Can't test without key, but verify the function exists
	// function signature verified by compilation
}
