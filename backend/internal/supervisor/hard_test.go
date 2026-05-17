// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package supervisor

import (
	"testing"
	"time"
)

func TestHard_Supervisor_ConfigValidation(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.AuditInterval <= 0 { t.Error("audit interval") }
	if cfg.HeartbeatTTL <= 0 { t.Error("heartbeat TTL") }
	if cfg.ResponseTimeout <= 0 { t.Error("response timeout") }
	if cfg.BaseSampleLow < 0 || cfg.BaseSampleLow > 1 { t.Error("sample low") }
	if cfg.BaseSampleMedium < cfg.BaseSampleLow { t.Error("sample medium < low") }
	if cfg.BaseSampleHigh < cfg.BaseSampleMedium { t.Error("sample high < medium") }
}

func TestHard_Supervisor_Lifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AuditInterval = 100 * time.Millisecond
	s := NewSupervisor(nil, nil, cfg, "prime")
	if s == nil { t.Fatal("nil") }
	s.Stop() // stop before start — should not panic
}

func TestHard_Supervisor_EvalResults(t *testing.T) {
	results := []EvalResult{
		{Quality: "good", Issues: nil},
		{Quality: "degraded", Issues: []string{"slow response"}},
		{Quality: "bad", Issues: []string{"wrong answer", "hallucination"}},
	}
	for _, r := range results {
		if r.Quality == "" { t.Error("empty quality") }
		if r.Quality == "bad" && len(r.Issues) == 0 { t.Error("bad with no issues") }
	}
}
