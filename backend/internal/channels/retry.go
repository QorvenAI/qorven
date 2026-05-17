// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Shared retry + error classification for all channels.
// Every channel should use this instead of raw API calls.

func RetrySend(channelName string, maxAttempts int, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err := fn()
		if err == nil { return nil }
		lastErr = err
		if IsPermError(err) { return err }
		delay := time.Duration(500*(1<<attempt)) * time.Millisecond
		if delay > 10*time.Second { delay = 10 * time.Second }
		slog.Warn("channel.send.retry", "channel", channelName, "attempt", attempt+1, "delay", delay, "error", err)
		time.Sleep(delay)
	}
	return fmt.Errorf("%s: failed after %d retries: %w", channelName, maxAttempts, lastErr)
}

func IsRetryError(err error) bool {
	if err == nil { return false }
	msg := strings.ToLower(err.Error())
	for _, p := range []string{"timeout", "connection reset", "eof", "429", "too many", "500", "502", "503", "temporarily", "try again", "broken pipe"} {
		if strings.Contains(msg, p) { return true }
	}
	return false
}

func IsPermError(err error) bool {
	if err == nil { return false }
	msg := strings.ToLower(err.Error())
	for _, p := range []string{"401", "403", "forbidden", "unauthorized", "invalid token", "revoked", "not found", "blocked", "banned"} {
		if strings.Contains(msg, p) { return true }
	}
	return false
}
