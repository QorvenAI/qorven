// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

package gateway

// capability_gaps.go — detects missing capabilities (starting with unknown
// model pricing) and routes them to Prime so it can self-resolve or ask the user.
//
// Design:
//   - CapabilityGapReporter is a thin shim between CostLedger and the supervisor bus.
//   - Each unique (gap_type, subject) is reported at most once per reportInterval
//     (default 1h) — no spam if the same unknown model is called 1000 times.
//   - Prime receives a CAPABILITY_GAP message with structured Context so it can
//     decide: look up the price autonomously, or escalate to the user.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	supervisorpkg "github.com/qorvenai/qorven/internal/supervisor"
)

// reportInterval is the minimum time between repeated reports for the same gap.
const reportInterval = time.Hour

// GapType classifies what kind of capability is missing.
type GapType string

const (
	GapTypePricing GapType = "model_pricing" // model price unknown — can't track spend
)

// CapabilityGap describes a single missing capability the system detected.
type CapabilityGap struct {
	Type      GapType        `json:"type"`
	Subject   string         `json:"subject"`    // e.g. model ID "gpt-5"
	Detail    string         `json:"detail"`     // human-readable description
	Context   map[string]any `json:"context"`    // structured data for Prime to act on
	DetectedAt time.Time     `json:"detected_at"`
}

// CapabilityGapReporter debounces and routes capability gaps to Prime
// via the supervisor bus. It is safe to use from multiple goroutines.
type CapabilityGapReporter struct {
	bus      *supervisorpkg.Bus
	primeID  string

	mu       sync.Mutex
	lastSent map[string]time.Time // "type:subject" → last report time
}

// NewCapabilityGapReporter creates a reporter wired to the given supervisor bus and Prime agent.
func NewCapabilityGapReporter(bus *supervisorpkg.Bus, primeID string) *CapabilityGapReporter {
	return &CapabilityGapReporter{
		bus:      bus,
		primeID:  primeID,
		lastSent: make(map[string]time.Time),
	}
}

// Report fires a CAPABILITY_GAP message to Prime if the same gap hasn't been
// reported within reportInterval. Non-blocking — logs and returns on failure.
func (r *CapabilityGapReporter) Report(ctx context.Context, gap CapabilityGap) {
	if r == nil || r.bus == nil || r.primeID == "" {
		return
	}

	key := fmt.Sprintf("%s:%s", gap.Type, gap.Subject)

	r.mu.Lock()
	last, seen := r.lastSent[key]
	if seen && time.Since(last) < reportInterval {
		r.mu.Unlock()
		return // debounce — already reported recently
	}
	r.lastSent[key] = time.Now()
	r.mu.Unlock()

	ctx3 := map[string]any{}
	for k, v := range gap.Context {
		ctx3[k] = v
	}
	ctx3["gap_type"] = string(gap.Type)
	ctx3["subject"]  = gap.Subject

	msg := supervisorpkg.Message{
		ID:        uuid.New().String(),
		From:      "system",
		To:        r.primeID,
		Intent:    supervisorpkg.IntentCapabilityGap,
		Content:   gap.Detail,
		Context:   ctx3,
		Risk:      supervisorpkg.RiskLow,
		Timestamp: time.Now(),
	}

	if err := r.bus.Send(ctx, msg); err != nil {
		slog.Warn("capability_gap.send_failed",
			"type", gap.Type, "subject", gap.Subject, "err", err)
	} else {
		slog.Info("capability_gap.reported",
			"type", gap.Type, "subject", gap.Subject, "prime", r.primeID)
	}
}

// ReportPricingGap is a convenience wrapper for model pricing gaps.
func (r *CapabilityGapReporter) ReportPricingGap(ctx context.Context, modelID, providerID string, tokensIn, tokensOut int) {
	detail := fmt.Sprintf(
		"Model %q has no pricing data — %d input + %d output tokens were recorded at $0 cost. "+
			"Please find the correct price (input $/1M, output $/1M) so past and future calls can be accounted for accurately.",
		modelID, tokensIn, tokensOut,
	)
	r.Report(ctx, CapabilityGap{
		Type:    GapTypePricing,
		Subject: modelID,
		Detail:  detail,
		Context: map[string]any{
			"model_id":    modelID,
			"provider_id": providerID,
			"tokens_in":   tokensIn,
			"tokens_out":  tokensOut,
			"action":      "set_price",
			"api_hint":    fmt.Sprintf("PUT /v1/gateway/pricing/%s  {\"input_per_1m\": X, \"output_per_1m\": Y}", modelID),
		},
		DetectedAt: time.Now(),
	})
}
