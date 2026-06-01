// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// unmarshalList parses a JSON response that contains a list.
// Handles both bare arrays and {"key": [...]} envelopes.
func unmarshalList(data json.RawMessage) []map[string]any {
	var list []map[string]any
	if json.Unmarshal(data, &list) == nil {
		return list
	}
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

func unmarshalMap(data json.RawMessage) map[string]any {
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

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

// ── Auth profile stubs (auth CLI removed — web UI handles login) ──────────────

type profilesFile struct {
	Active   string    `json:"active"`
	Profiles []profile `json:"profiles"`
}
type profile struct {
	Name   string `json:"name"`
	Server string `json:"server"`
	Token  string `json:"token"`
}

func loadProfiles() (profilesFile, error) { return profilesFile{}, nil }
func loadToken(_ string) string           { return "" }

// ── Crypto helpers (used by init command) ─────────────────────────────────────

func generateToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		panic("generateToken: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func writeEnvFile(path, dsn, token, encKey string) {
	content := strings.Join([]string{
		"# Qorven secrets — keep private",
		"QORVEN_POSTGRES_DSN=" + dsn,
		"QORVEN_GATEWAY_TOKEN=" + token,
		"QORVEN_ENCRYPTION_KEY=" + encKey,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		fmt.Printf("  WARN: could not write %s: %v\n", path, err)
	}
}
