// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"strings"
	"testing"

	"github.com/qorvenai/qorven/internal/providers"
)

func TestMaybeInjectOverride(t *testing.T) {
	base := []providers.Message{
		{Role: "user", Content: "hello"},
	}

	t.Run("nil fn returns unchanged", func(t *testing.T) {
		got := maybeInjectOverride(nil)
		if got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("fn returning empty string leaves messages unchanged", func(t *testing.T) {
		fn := func() string { return "" }
		ov := maybeInjectOverride(fn)
		if ov != "" {
			t.Fatalf("expected empty string, got %q", ov)
		}
		// messages unchanged (caller only appends when non-empty)
		if len(base) != 1 {
			t.Fatalf("expected 1 message, got %d", len(base))
		}
	})

	t.Run("fn returning message is returned as-is", func(t *testing.T) {
		fn := func() string { return "do X" }
		ov := maybeInjectOverride(fn)
		if ov != "do X" {
			t.Fatalf("expected %q, got %q", "do X", ov)
		}
		// Simulate what the loop does: prepend prefix and append
		msgs := append(base, providers.Message{Role: "user", Content: "[User override] " + ov})
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages after inject, got %d", len(msgs))
		}
		last := msgs[len(msgs)-1]
		if last.Role != "user" {
			t.Fatalf("expected role user, got %q", last.Role)
		}
		if !strings.Contains(last.Content, "do X") {
			t.Fatalf("expected content to contain %q, got %q", "do X", last.Content)
		}
	})
}
