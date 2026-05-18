// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"strings"
)

// errors.go — User-friendly error formatting.
// Never show raw Go errors. Always show what to do next.

// userError wraps an error with helpful context and suggestions.
type userError struct {
	message     string
	suggestions []string
}

func (e *userError) Error() string {
	var b strings.Builder
	b.WriteString("\n  ✗ " + e.message + "\n")
	if len(e.suggestions) > 0 {
		b.WriteString("\n")
		for _, s := range e.suggestions {
			b.WriteString("    → " + s + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// errNoConfig returns when config file is missing.
func errNoConfig() error {
	return &userError{
		message: "Configuration not found",
		suggestions: []string{
			"Run: qorven init",
			"Or create config manually: qorven config edit",
		},
	}
}

// errNoDB returns when database is not configured or unreachable.
func errNoDB(detail string) error {
	return &userError{
		message: fmt.Sprintf("Cannot connect to database: %s", detail),
		suggestions: []string{
			"Is PostgreSQL running? systemctl status postgresql",
			"Check your DSN: qorven config show",
			"Run setup wizard: qorven init",
		},
	}
}

// errNoGateway returns when gateway is not running.
func errNoGateway() error {
	return &userError{
		message: "Gateway is not running",
		suggestions: []string{
			"Start it: qorven start",
			"Or in background: qorven gateway start",
			"Check status: qorven gateway status",
		},
	}
}

// errNoProvider returns when no LLM provider is configured.
func errNoProvider() error {
	return &userError{
		message: "No LLM provider configured",
		suggestions: []string{
			"Add a provider: qorven config set providers.deepseek.api_key YOUR_KEY",
			"Or run setup: qorven init",
		},
	}
}

// errNoAgent returns when no agents exist.
func errNoAgent() error {
	return &userError{
		message: "No agents found",
		suggestions: []string{
			"Create one: qorven agent create --key my-agent --model gpt-4o-mini",
			"Or run setup: qorven init",
		},
	}
}

// errAuth returns when authentication fails.
func errAuth() error {
	return &userError{
		message: "Authentication failed",
		suggestions: []string{
			"Set token: export QORVEN_TOKEN=your-token",
			"Or check config: qorven config show",
			"Token is shown during: qorven init",
		},
	}
}
