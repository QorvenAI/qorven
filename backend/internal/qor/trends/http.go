// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// http.go — HTTP client with retries, rate limiting, and error handling.

const (
	DefaultTimeout    = 30 * time.Second
	MaxRetries        = 5
	Max429Retries     = 2
	RetryBaseDelay    = 2 * time.Second
	UserAgent         = "Qorven/1.0 (Social Intelligence)"
	MaxResponseBytes  = 4 << 20 // 4MB
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient(timeout time.Duration) *HTTPClient {
	if timeout <= 0 { timeout = DefaultTimeout }
	return &HTTPClient{client: &http.Client{Timeout: timeout}}
}

type HTTPError struct {
	StatusCode int
	Body       string
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// Get performs a GET request with retries and exponential backoff.
func (c *HTTPClient) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	return c.Do(ctx, "GET", url, headers, nil)
}

// Do performs an HTTP request with retries.
func (c *HTTPClient) Do(ctx context.Context, method, url string, headers map[string]string, body io.Reader) ([]byte, error) {
	var lastErr error
	retries429 := 0

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			delay := RetryBaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			jitter := time.Duration(rand.Int63n(int64(delay / 4)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, body)
		if err != nil { return nil, err }
		req.Header.Set("User-Agent", UserAgent)
		for k, v := range headers { req.Header.Set(k, v) }

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
		resp.Body.Close()

		if resp.StatusCode == 429 {
			retries429++
			if retries429 > Max429Retries {
				return nil, &HTTPError{StatusCode: 429, Body: string(respBody), Message: "rate limited"}
			}
			lastErr = &HTTPError{StatusCode: 429, Body: string(respBody), Message: "rate limited"}
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), Message: "server error"}
			continue
		}

		if resp.StatusCode >= 400 {
			return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody),
				Message: fmt.Sprintf("%s %s", method, url)}
		}

		return respBody, nil
	}

	if lastErr != nil { return nil, lastErr }
	return nil, fmt.Errorf("max retries exceeded for %s %s", method, url)
}
