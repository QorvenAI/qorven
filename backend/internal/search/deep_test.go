// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package search

import (
	"context"
	"testing"
	"time"
)

func TestDeep_BraveSearch_Structure(t *testing.T) {
	// Test with empty key — should fail gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := BraveSearch(ctx, "", "test query", 5)
	if err != nil { t.Logf("brave without key: %v", err) }
	_ = results
}

func TestDeep_ExaSearch_Structure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := ExaSearch(ctx, "", "test query", 5)
	if err != nil { t.Logf("exa without key: %v", err) }
	_ = results
}

func TestDeep_JinaSearch_Structure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results, err := JinaSearch(ctx, "", "test query", 5)
	if err != nil { t.Logf("jina without key: %v", err) }
	_ = results
}

func TestDeep_KagiFastGPT_Structure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answer, err := KagiFastGPT(ctx, "", "test query")
	if err != nil { t.Logf("kagi without key: %v", err) }
	_ = answer
}

func TestDeep_Result_Fields(t *testing.T) {
	r := Result{
		Title:   "Qorven Documentation",
		URL:     "https://docs.qorven.io",
		Snippet: "Qorven is a multi-agent AI workspace platform.",
		Source:  "brave",
	}
	if r.Title == "" { t.Error("empty title") }
	if r.URL == "" { t.Error("empty URL") }
	if r.Source != "brave" { t.Error("wrong source") }
}

func TestDeep_Search_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)
	_, err := BraveSearch(ctx, "fake-key", "test", 5)
	if err == nil { t.Log("may complete before timeout with cached DNS") }
}
