// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"testing"
	"encoding/json"
)

// Fuzz tests — find crashes with random input.

func FuzzNormalizeSchema(f *testing.F) {
	// Seed corpus
	f.Add("openai", `{"type":"object","properties":{"q":{"type":"string"}}}`)
	f.Add("anthropic", `{"$ref":"#/$defs/X","$defs":{"X":{"type":"string"}}}`)
	f.Add("gemini", `{"anyOf":[{"type":"string"},{"type":"null"}]}`)
	f.Add("xai", `{"const":"fixed"}`)
	f.Add("unknown", `{}`)
	f.Add("openai", `null`)

	f.Fuzz(func(t *testing.T, provider, schemaJSON string) {
		var schema map[string]any
		// Try to parse as JSON — if it fails, use empty map
		if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
			schema = map[string]any{}
		}
		// Should never panic
		NormalizeSchema(provider, schema)
	})
}

func FuzzIsRetryableError(f *testing.F) {
	f.Add("connection reset")
	f.Add("broken pipe")
	f.Add("EOF")
	f.Add("timeout")
	f.Add("invalid json")
	f.Add("")

	f.Fuzz(func(t *testing.T, errMsg string) {
		IsRetryableError(errStr(errMsg))
	})
}

func FuzzParseRetryAfter(f *testing.F) {
	f.Add("5")
	f.Add("60")
	f.Add("")
	f.Add("abc")
	f.Add("Thu, 01 Dec 1994 16:00:00 GMT")

	f.Fuzz(func(t *testing.T, value string) {
		ParseRetryAfter(value)
	})
}

func FuzzNormalizeReasoningEffort(f *testing.F) {
	f.Add("off")
	f.Add("auto")
	f.Add("high")
	f.Add("")
	f.Add("INVALID")

	f.Fuzz(func(t *testing.T, effort string) {
		NormalizeReasoningEffort(effort)
	})
}
