// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"strings"
	"time"
)

// ChannelHealthState captures the current runtime state of a channel instance.
type ChannelHealthState string

const (
	ChannelHealthStateRegistered ChannelHealthState = "registered"
	ChannelHealthStateStarting   ChannelHealthState = "starting"
	ChannelHealthStateHealthy    ChannelHealthState = "healthy"
	ChannelHealthStateDegraded   ChannelHealthState = "degraded"
	ChannelHealthStateFailed     ChannelHealthState = "failed"
	ChannelHealthStateStopped    ChannelHealthState = "stopped"
)

// ChannelFailureKind classifies the dominant cause of the current failure state.
type ChannelFailureKind string

const (
	ChannelFailureKindAuth    ChannelFailureKind = "auth"
	ChannelFailureKindConfig  ChannelFailureKind = "config"
	ChannelFailureKindNetwork ChannelFailureKind = "network"
	ChannelFailureKindUnknown ChannelFailureKind = "unknown"
)

// ChannelRemediation contains operator hints for the current incident.
type ChannelRemediation struct {
	Code     string `json:"code"`
	Headline string `json:"headline"`
	Hint     string `json:"hint,omitempty"`
	Target   string `json:"target,omitempty"`
}

// ChannelHealth is the shared runtime health snapshot.
type ChannelHealth struct {
	ChannelType         string              `json:"-"`
	Enabled             bool                `json:"enabled"`
	Running             bool                `json:"running"`
	State               ChannelHealthState  `json:"state"`
	Summary             string              `json:"summary,omitempty"`
	Detail              string              `json:"detail,omitempty"`
	FailureKind         ChannelFailureKind  `json:"failure_kind,omitempty"`
	Retryable           bool                `json:"retryable"`
	CheckedAt           time.Time           `json:"checked_at,omitempty"`
	FailureCount        int                 `json:"failure_count,omitempty"`
	ConsecutiveFailures int                 `json:"consecutive_failures,omitempty"`
	FirstFailedAt       time.Time           `json:"first_failed_at,omitempty"`
	LastFailedAt        time.Time           `json:"last_failed_at,omitempty"`
	LastHealthyAt       time.Time           `json:"last_healthy_at,omitempty"`
	Remediation         *ChannelRemediation `json:"remediation,omitempty"`
}

// ChannelErrorInfo contains shared error classification output.
type ChannelErrorInfo struct {
	Summary   string
	Detail    string
	Kind      ChannelFailureKind
	Retryable bool
}

// ClassifyChannelError maps common channel failures into operator-facing buckets.
func ClassifyChannelError(err error) ChannelErrorInfo {
	if err == nil {
		return ChannelErrorInfo{
			Summary:   "Channel failed",
			Detail:    "Could not determine the latest channel error.",
			Kind:      ChannelFailureKindUnknown,
			Retryable: true,
		}
	}

	msg := strings.ToLower(err.Error())

	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden"):
		return ChannelErrorInfo{
			Summary:   "Authentication failed",
			Detail:    "The upstream service rejected the configured credentials.",
			Kind:      ChannelFailureKindAuth,
			Retryable: false,
		}
	case strings.Contains(msg, "token is required") || strings.Contains(msg, "missing credentials") || strings.Contains(msg, "required"):
		return ChannelErrorInfo{
			Summary:   "Configuration is invalid",
			Detail:    "Required channel credentials are missing or incomplete.",
			Kind:      ChannelFailureKindConfig,
			Retryable: false,
		}
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return ChannelErrorInfo{
			Summary:   "Network error",
			Detail:    "Timed out while reaching the upstream service.",
			Kind:      ChannelFailureKindNetwork,
			Retryable: true,
		}
	case strings.Contains(msg, "connection refused"):
		return ChannelErrorInfo{
			Summary:   "Network error",
			Detail:    "The upstream service refused the connection.",
			Kind:      ChannelFailureKindNetwork,
			Retryable: true,
		}
	case strings.Contains(msg, "no such host"):
		return ChannelErrorInfo{
			Summary:   "Network error",
			Detail:    "Could not resolve the upstream host.",
			Kind:      ChannelFailureKindNetwork,
			Retryable: true,
		}
	case strings.Contains(msg, "connection reset") || strings.Contains(msg, "eof"):
		return ChannelErrorInfo{
			Summary:   "Network error",
			Detail:    "The upstream service closed the connection unexpectedly.",
			Kind:      ChannelFailureKindNetwork,
			Retryable: true,
		}
	default:
		return ChannelErrorInfo{
			Summary:   "Channel failed",
			Detail:    "An unexpected channel error occurred.",
			Kind:      ChannelFailureKindUnknown,
			Retryable: true,
		}
	}
}

// NewChannelHealth builds a runtime snapshot with current timestamp.
func NewChannelHealth(state ChannelHealthState, summary, detail string, kind ChannelFailureKind, retryable bool) ChannelHealth {
	return ChannelHealth{
		Enabled:     true,
		Running:     state == ChannelHealthStateHealthy || state == ChannelHealthStateDegraded,
		State:       state,
		Summary:     summary,
		Detail:      detail,
		FailureKind: kind,
		Retryable:   retryable,
		CheckedAt:   time.Now().UTC(),
	}
}

// NewFailedChannelHealth builds a failed snapshot from a classified error.
func NewFailedChannelHealth(summary string, err error) ChannelHealth {
	info := ClassifyChannelError(err)
	if summary == "" {
		summary = info.Summary
	}
	return NewChannelHealth(ChannelHealthStateFailed, summary, info.Detail, info.Kind, info.Retryable)
}

func isFailureState(state ChannelHealthState) bool {
	return state == ChannelHealthStateFailed || state == ChannelHealthStateDegraded
}

func mergeChannelHealth(previous, snapshot ChannelHealth) ChannelHealth {
	if snapshot.CheckedAt.IsZero() {
		snapshot.CheckedAt = time.Now().UTC()
	}
	if !snapshot.Enabled {
		snapshot.Enabled = true
	}
	if snapshot.ChannelType == "" {
		snapshot.ChannelType = previous.ChannelType
	}

	if isFailureState(snapshot.State) {
		if snapshot.FailureCount == 0 {
			snapshot.FailureCount = previous.FailureCount + 1
		}
		if snapshot.ConsecutiveFailures == 0 {
			snapshot.ConsecutiveFailures = previous.ConsecutiveFailures + 1
		}
		if snapshot.FirstFailedAt.IsZero() {
			if previous.FirstFailedAt.IsZero() || !isFailureState(previous.State) {
				snapshot.FirstFailedAt = snapshot.CheckedAt
			} else {
				snapshot.FirstFailedAt = previous.FirstFailedAt
			}
		}
		if snapshot.LastFailedAt.IsZero() {
			snapshot.LastFailedAt = snapshot.CheckedAt
		}
		if snapshot.LastHealthyAt.IsZero() {
			snapshot.LastHealthyAt = previous.LastHealthyAt
		}
	} else {
		if snapshot.FailureCount == 0 {
			snapshot.FailureCount = previous.FailureCount
		}
		snapshot.ConsecutiveFailures = 0
		snapshot.FirstFailedAt = time.Time{}
		if snapshot.LastFailedAt.IsZero() {
			snapshot.LastFailedAt = previous.LastFailedAt
		}
		if snapshot.State == ChannelHealthStateHealthy {
			snapshot.LastHealthyAt = snapshot.CheckedAt
		} else if snapshot.LastHealthyAt.IsZero() {
			snapshot.LastHealthyAt = previous.LastHealthyAt
		}
	}

	snapshot.Remediation = buildChannelRemediation(snapshot)
	return snapshot
}

func buildChannelRemediation(snapshot ChannelHealth) *ChannelRemediation {
	if !isFailureState(snapshot.State) {
		return nil
	}

	text := strings.ToLower(snapshot.Summary + " " + snapshot.Detail)

	switch snapshot.FailureKind {
	case ChannelFailureKindAuth:
		return &ChannelRemediation{
			Code:     "open_credentials",
			Headline: "Review channel credentials",
			Hint:     "Open credentials and confirm the current token or secret is still valid.",
			Target:   "credentials",
		}
	case ChannelFailureKindConfig:
		if strings.Contains(text, "credential") || strings.Contains(text, "token") || strings.Contains(text, "required") {
			return &ChannelRemediation{
				Code:     "open_credentials",
				Headline: "Complete required credentials",
				Hint:     "Open credentials and fill the missing or invalid values.",
				Target:   "credentials",
			}
		}
		return &ChannelRemediation{
			Code:     "open_advanced",
			Headline: "Review channel settings",
			Hint:     "Open advanced settings and correct the invalid configuration.",
			Target:   "advanced",
		}
	case ChannelFailureKindNetwork:
		return &ChannelRemediation{
			Code:     "check_network",
			Headline: "Check upstream reachability",
			Hint:     "Verify the upstream service is reachable, then inspect proxy settings if used.",
			Target:   "details",
		}
	default:
		return &ChannelRemediation{
			Code:     "open_advanced",
			Headline: "Review channel settings",
			Hint:     "Open channel settings and inspect the latest error detail.",
			Target:   "advanced",
		}
	}
}
