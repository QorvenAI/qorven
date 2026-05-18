// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// unmarshalList parses a JSON response that contains a list.
// Handles both bare arrays and {"key": [...]} envelopes.
func unmarshalList(data json.RawMessage) []map[string]any {
	// Try bare array first
	var list []map[string]any
	if json.Unmarshal(data, &list) == nil {
		return list
	}
	// Try envelope: {"agents": [...], "sessions": [...], etc.}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(data, &envelope) == nil {
		for _, v := range envelope {
			if json.Unmarshal(v, &list) == nil && len(list) > 0 {
				return list
			}
		}
	}
	return nil
}

// unmarshalMap parses a JSON response as a single object.
func unmarshalMap(data json.RawMessage) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

// str safely gets a string from a map.
func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// buildBody creates a map from key-value pairs, skipping empty/zero values.
func buildBody(pairs ...any) map[string]any {
	body := make(map[string]any)
	for i := 0; i < len(pairs)-1; i += 2 {
		key := pairs[i].(string)
		val := pairs[i+1]
		switch v := val.(type) {
		case string:
			if v != "" {
				body[key] = v
			}
		case int:
			if v != 0 {
				body[key] = v
			}
		case bool:
			body[key] = v
		default:
			if v != nil {
				body[key] = v
			}
		}
	}
	return body
}

// readContent reads from @file or returns literal string.
func readContent(val string) (string, error) {
	if strings.HasPrefix(val, "@") {
		data, err := os.ReadFile(val[1:])
		if err != nil {
			return "", fmt.Errorf("read file %s: %w", val[1:], err)
		}
		return string(data), nil
	}
	return val, nil
}
func mmin(a, b int) int { if a < b { return a }; return b }

func safeID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
