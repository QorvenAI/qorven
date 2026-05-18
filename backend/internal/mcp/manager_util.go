// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package mcp

import (
	"encoding/json"
	"strings"
	"os"
)

// mapToEnvSlice converts a map of environment variables to a slice of "key=value" strings.
func mapToEnvSlice(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}

// toSet converts a slice of strings to a set (map for O(1) lookup).
func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	s := make(map[string]struct{}, len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

// joinErrors joins multiple error strings with "; " separator.
func joinErrors(errs []string) string {
	var result strings.Builder
	for i, e := range errs {
		if i > 0 {
			result.WriteString("; ")
		}
		result.WriteString(e)
	}
	return result.String()
}

// ParseJSONBytesToStringSlice converts JSONB []byte to []string (exported for loop_mcp_user).
func ParseJSONBytesToStringSlice(data []byte) []string {
	return jsonBytesToStringSlice(data)
}

// ParseJSONBytesToStringMap converts JSONB []byte to map[string]string (exported for loop_mcp_user).
func ParseJSONBytesToStringMap(data []byte) map[string]string {
	return jsonBytesToStringMap(data)
}

// jsonBytesToStringSlice converts JSONB []byte to []string. Returns nil on error.
func jsonBytesToStringSlice(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	result := []string{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// jsonBytesToStringMap converts JSONB []byte to map[string]string. Returns nil on error.
func jsonBytesToStringMap(data []byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// resolveEnvVars replaces "env:VAR_NAME" values with actual environment variables.
func resolveEnvVars(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if after, ok := strings.CutPrefix(v, "env:"); ok {
			out[k] = os.Getenv(after)
		} else {
			out[k] = v
		}
	}
	return out
}

// requireUserCreds checks if an MCP server's settings mandate per-user credentials.
func requireUserCreds(settings json.RawMessage) bool {
	if len(settings) == 0 {
		return false
	}
	var s struct {
		RequireUserCredentials bool `json:"require_user_credentials"`
	}
	_ = json.Unmarshal(settings, &s)
	return s.RequireUserCredentials
}
