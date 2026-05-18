// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"
)

// RetryConfig configures retry behavior for provider HTTP requests.
type RetryConfig struct {
	Attempts int           // max attempts (default 3, 1 = no retry)
	MinDelay time.Duration // initial delay (default 300ms)
	MaxDelay time.Duration // delay cap (default 30s)
	Jitter   float64       // jitter factor ±N (default 0.1 = ±10%)
}

// RetryHookFunc is called before each retry attempt for UI feedback.
type RetryHookFunc func(attempt, maxAttempts int, err error)

type retryHookKey struct{}

// WithRetryHook injects a retry notification callback into the context.
func WithRetryHook(ctx context.Context, fn RetryHookFunc) context.Context {
	return context.WithValue(ctx, retryHookKey{}, fn)
}

func retryHookFromContext(ctx context.Context) RetryHookFunc {
	fn, _ := ctx.Value(retryHookKey{}).(RetryHookFunc)
	return fn
}

// DefaultRetryConfig returns sensible defaults for LLM provider calls.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{Attempts: 3, MinDelay: 300 * time.Millisecond, MaxDelay: 30 * time.Second, Jitter: 0.1}
}

// HTTPError represents an HTTP error with status code and optional Retry-After.
type HTTPError struct {
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Body) }

// IsRetryableError checks if an error is retryable (429, 5xx, network errors, timeouts).
func IsRetryableError(err error) bool {
	if err == nil { return false }

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case 429, 500, 502, 503, 504:
			return true
		}
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) { return true }

	errStr := err.Error()
	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "timeout")
}

// RetryDo executes fn with exponential backoff, jitter, and Retry-After support.
func RetryDo[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	if cfg.Attempts <= 0 { cfg.Attempts = 1 }

	var lastErr error
	var zero T

	for attempt := 1; attempt <= cfg.Attempts; attempt++ {
		result, err := fn()
		if err == nil { return result, nil }
		lastErr = err

		if !IsRetryableError(err) || attempt == cfg.Attempts {
			return zero, err
		}

		delay := computeRetryDelay(cfg, attempt, err)
		slog.Debug("provider.retry", "attempt", attempt, "max", cfg.Attempts, "delay", delay, "err", err.Error())

		if hook := retryHookFromContext(ctx); hook != nil {
			hook(attempt, cfg.Attempts, err)
		}

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}
	return zero, lastErr
}

func computeRetryDelay(cfg RetryConfig, attempt int, err error) time.Duration {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
		return httpErr.RetryAfter
	}

	delay := float64(cfg.MinDelay) * math.Pow(2, float64(attempt-1))
	if time.Duration(delay) > cfg.MaxDelay { delay = float64(cfg.MaxDelay) }
	if cfg.Jitter > 0 {
		delay += (rand.Float64()*2 - 1) * delay * cfg.Jitter
	}
	if delay < 0 { delay = float64(cfg.MinDelay) }
	return time.Duration(delay)
}

// ParseRetryAfter parses a Retry-After header value (seconds or HTTP-date).
func ParseRetryAfter(value string) time.Duration {
	if value == "" { return 0 }
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, value); err == nil {
		if d := time.Until(t); d > 0 { return d }
	}
	return 0
}
