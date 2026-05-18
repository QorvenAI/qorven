// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package providers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.Attempts != 3 { t.Errorf("attempts=%d, want 3", cfg.Attempts) }
	if cfg.MinDelay != 300*time.Millisecond { t.Errorf("minDelay=%v", cfg.MinDelay) }
	if cfg.MaxDelay != 30*time.Second { t.Errorf("maxDelay=%v", cfg.MaxDelay) }
}

func TestIsRetryableError_Nil(t *testing.T) {
	if IsRetryableError(nil) { t.Error("nil should not be retryable") }
}

func TestIsRetryableError_HTTPError429(t *testing.T) {
	err := &HTTPError{Status: 429, Body: "rate limited"}
	if !IsRetryableError(err) { t.Error("429 should be retryable") }
}

func TestIsRetryableError_HTTPError500(t *testing.T) {
	if !IsRetryableError(&HTTPError{Status: 500}) { t.Error("500 should be retryable") }
}

func TestIsRetryableError_HTTPError502(t *testing.T) {
	if !IsRetryableError(&HTTPError{Status: 502}) { t.Error("502 should be retryable") }
}

func TestIsRetryableError_HTTPError503(t *testing.T) {
	if !IsRetryableError(&HTTPError{Status: 503}) { t.Error("503 should be retryable") }
}

func TestIsRetryableError_HTTPError400(t *testing.T) {
	if IsRetryableError(&HTTPError{Status: 400}) { t.Error("400 should NOT be retryable") }
}

func TestIsRetryableError_HTTPError401(t *testing.T) {
	if IsRetryableError(&HTTPError{Status: 401}) { t.Error("401 should NOT be retryable") }
}

func TestIsRetryableError_ConnectionReset(t *testing.T) {
	if !IsRetryableError(errors.New("connection reset by peer")) { t.Error("connection reset should be retryable") }
}

func TestIsRetryableError_Timeout(t *testing.T) {
	if !IsRetryableError(errors.New("context deadline exceeded (timeout)")) { t.Error("timeout should be retryable") }
}

func TestIsRetryableError_EOF(t *testing.T) {
	if !IsRetryableError(errors.New("unexpected EOF")) { t.Error("EOF should be retryable") }
}

func TestIsRetryableError_RegularError(t *testing.T) {
	if IsRetryableError(errors.New("invalid json")) { t.Error("regular error should NOT be retryable") }
}

func TestRetryDo_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	result, err := RetryDo(context.Background(), DefaultRetryConfig(), func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Errorf("result=%q", result) }
	if calls != 1 { t.Errorf("calls=%d, want 1", calls) }
}

func TestRetryDo_SuccessAfterRetry(t *testing.T) {
	calls := 0
	cfg := RetryConfig{Attempts: 3, MinDelay: 1 * time.Millisecond, MaxDelay: 10 * time.Millisecond}
	result, err := RetryDo(context.Background(), cfg, func() (string, error) {
		calls++
		if calls < 3 { return "", &HTTPError{Status: 429, Body: "rate limited"} }
		return "ok", nil
	})
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Errorf("result=%q", result) }
	if calls != 3 { t.Errorf("calls=%d, want 3", calls) }
}

func TestRetryDo_AllAttemptsFail(t *testing.T) {
	cfg := RetryConfig{Attempts: 2, MinDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	_, err := RetryDo(context.Background(), cfg, func() (string, error) {
		return "", &HTTPError{Status: 500, Body: "server error"}
	})
	if err == nil { t.Fatal("expected error") }
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) { t.Fatal("expected HTTPError") }
	if httpErr.Status != 500 { t.Errorf("status=%d", httpErr.Status) }
}

func TestRetryDo_NonRetryableError_NoRetry(t *testing.T) {
	calls := 0
	cfg := RetryConfig{Attempts: 3, MinDelay: 1 * time.Millisecond}
	_, err := RetryDo(context.Background(), cfg, func() (string, error) {
		calls++
		return "", &HTTPError{Status: 400, Body: "bad request"}
	})
	if err == nil { t.Fatal("expected error") }
	if calls != 1 { t.Errorf("calls=%d, want 1 (no retry for 400)", calls) }
}

func TestRetryDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := RetryConfig{Attempts: 3, MinDelay: 1 * time.Second}
	calls := 0
	_, err := RetryDo(ctx, cfg, func() (string, error) {
		calls++
		return "", &HTTPError{Status: 429}
	})
	if err == nil { t.Fatal("expected error") }
	if calls > 3 { t.Errorf("too many calls with cancelled context: %d", calls) }
}

func TestRetryDo_ZeroAttempts(t *testing.T) {
	cfg := RetryConfig{Attempts: 0}
	result, err := RetryDo(context.Background(), cfg, func() (string, error) { return "ok", nil })
	if err != nil { t.Fatal(err) }
	if result != "ok" { t.Errorf("result=%q", result) }
}

func TestRetryDo_RetryHookCalled(t *testing.T) {
	hookCalls := 0
	ctx := WithRetryHook(context.Background(), func(attempt, max int, err error) { hookCalls++ })
	cfg := RetryConfig{Attempts: 3, MinDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	calls := 0
	RetryDo(ctx, cfg, func() (string, error) {
		calls++
		if calls < 3 { return "", &HTTPError{Status: 429} }
		return "ok", nil
	})
	if hookCalls != 2 { t.Errorf("hookCalls=%d, want 2", hookCalls) }
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	d := ParseRetryAfter("5")
	if d != 5*time.Second { t.Errorf("got %v", d) }
}

func TestParseRetryAfter_Empty(t *testing.T) {
	if ParseRetryAfter("") != 0 { t.Error("empty should return 0") }
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	if ParseRetryAfter("not-a-number") != 0 { t.Error("invalid should return 0") }
}

func TestHTTPError_Error(t *testing.T) {
	err := &HTTPError{Status: 429, Body: "too many"}
	if err.Error() != "HTTP 429: too many" { t.Errorf("got %q", err.Error()) }
}

func TestComputeRetryDelay_ExponentialBackoff(t *testing.T) {
	cfg := RetryConfig{MinDelay: 100 * time.Millisecond, MaxDelay: 10 * time.Second, Jitter: 0}
	d1 := computeRetryDelay(cfg, 1, errors.New("err"))
	d2 := computeRetryDelay(cfg, 2, errors.New("err"))
	d3 := computeRetryDelay(cfg, 3, errors.New("err"))
	if d2 <= d1 { t.Errorf("d2 (%v) should be > d1 (%v)", d2, d1) }
	if d3 <= d2 { t.Errorf("d3 (%v) should be > d2 (%v)", d3, d2) }
}

func TestComputeRetryDelay_RespectsRetryAfter(t *testing.T) {
	cfg := RetryConfig{MinDelay: 100 * time.Millisecond, MaxDelay: 10 * time.Second}
	err := &HTTPError{Status: 429, RetryAfter: 7 * time.Second}
	d := computeRetryDelay(cfg, 1, err)
	if d != 7*time.Second { t.Errorf("should use Retry-After: got %v", d) }
}

func TestComputeRetryDelay_CapsAtMax(t *testing.T) {
	cfg := RetryConfig{MinDelay: 1 * time.Second, MaxDelay: 2 * time.Second, Jitter: 0}
	d := computeRetryDelay(cfg, 10, errors.New("err"))
	if d > cfg.MaxDelay { t.Errorf("delay %v exceeds max %v", d, cfg.MaxDelay) }
}
