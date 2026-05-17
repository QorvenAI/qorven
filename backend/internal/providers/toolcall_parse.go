// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

package providers

import (
	"encoding/json"
	"log/slog"
)

// parseToolArgs decodes a tool-call's JSON arguments string. Returns
// the parsed map plus a boolean indicating whether the JSON parsed
// cleanly.
//
// Why this exists: a provider's streaming response can truncate mid-
// JSON (network blip, token-limit cutoff, buggy upstream adapter).
// The raw pattern `json.Unmarshal(b, &args)` that used to be at every
// call site swallowed the error — truncated JSON parsed to an empty
// map, and the tool then executed with no arguments. For `write_file`
// or `exec`, executing a valid-name-but-empty-args call is actively
// dangerous: a wildcard glob, a rm without a target, a write to an
// unknown path.
//
// Callers should check the returned bool and either refuse the call
// or mark it failed upstream. Returning the partial map as well keeps
// the error path observable — callers can log what WAS parsed.
func parseToolArgs(raw []byte, toolName string) (map[string]any, bool) {
	if len(raw) == 0 {
		// An empty arg string is valid for tools that take zero params
		// (e.g. list_agents, noop). Returning ok=true with an empty
		// map is correct — the tool's own schema validation catches
		// missing required params.
		return map[string]any{}, true
	}
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		slog.Warn("providers.toolcall.parse_failed",
			"tool", toolName,
			"error", err.Error(),
			"len", len(raw),
			"hint", "likely truncated stream — tool will be rejected")
		// Return what we can (probably empty) plus ok=false so the
		// caller routes this to a failed-tool-call response instead of
		// executing with bogus args.
		return map[string]any{}, false
	}
	if args == nil {
		// `json.Unmarshal("null", &m)` leaves m as nil. Normalise so
		// downstream code doesn't have to nil-guard every access.
		args = map[string]any{}
	}
	return args, true
}

// parseToolArgsString is the string-input convenience wrapper. Most
// provider adapters accumulate streaming JSON into a strings.Builder
// or similar before parsing.
func parseToolArgsString(raw string, toolName string) (map[string]any, bool) {
	return parseToolArgs([]byte(raw), toolName)
}
