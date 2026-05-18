// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package cron

import (
	"math/rand/v2"
	"time"
)

// RetryConfig controls exponential backoff retry for failed cron jobs.
type RetryConfig struct {
	MaxRetries int           // max retry attempts (default 3, 0 = no retry)
	BaseDelay  time.Duration // initial backoff delay (default 2s)
	MaxDelay   time.Duration // maximum backoff delay (default 30s)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{MaxRetries: 3, BaseDelay: 2 * time.Second, MaxDelay: 30 * time.Second}
}

// ExecuteWithRetry runs fn, retrying on error with exponential backoff + jitter.
func ExecuteWithRetry(fn func() (string, error), cfg RetryConfig) (result string, attempts int, err error) {
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		result, err = fn()
		if err == nil {
			return result, attempt + 1, nil
		}
		if attempt < cfg.MaxRetries {
			time.Sleep(backoffWithJitter(cfg.BaseDelay, cfg.MaxDelay, attempt))
		}
	}
	return "", cfg.MaxRetries + 1, err
}

// backoffWithJitter computes delay = min(base * 2^attempt, max) + jitter(±25%).
func backoffWithJitter(base, max time.Duration, attempt int) time.Duration {
	delay := min(base<<uint(attempt), max)
	quarter := delay / 4
	if quarter > 0 {
		jitter := time.Duration(rand.Int64N(int64(quarter*2))) - quarter
		delay += jitter
	}
	return delay
}

const maxOutputBytes = 16 * 1024

// TruncateOutput truncates output to maxOutputBytes.
func TruncateOutput(s string) string {
	if len(s) <= maxOutputBytes {
		return s
	}
	return s[:maxOutputBytes] + "...[truncated]"
}
