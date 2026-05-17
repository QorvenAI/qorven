// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defuddleDefaultBaseURL = "https://fetch.qorven.ai/"
	defuddleTimeout        = 10 * time.Second
	defuddleMaxBody        = 1 << 20 // 1MB
)

// DefuddleExtractor calls a Cloudflare Worker to extract clean markdown.
type DefuddleExtractor struct {
	baseURL string
	client  *http.Client
}

// NewDefuddleExtractorFromEntry creates a DefuddleExtractor from chain settings.
func NewDefuddleExtractorFromEntry(entry ExtractorEntry) *DefuddleExtractor {
	baseURL := entry.BaseURL
	if baseURL == "" {
		baseURL = defuddleDefaultBaseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	timeout := time.Duration(entry.Timeout) * time.Second
	if timeout <= 0 {
		timeout = defuddleTimeout
	}
	return &DefuddleExtractor{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				ForceAttemptHTTP2:   true,
				MaxIdleConns:        5,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (d *DefuddleExtractor) Name() string { return "defuddle" }

// Extract sends GET to baseURL/<domain>/<path> and returns markdown.
func (d *DefuddleExtractor) Extract(ctx context.Context, rawURL string) (string, error) {
	target := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	fetchURL := d.baseURL + target

	ctx, cancel := context.WithTimeout(ctx, d.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return "", fmt.Errorf("create defuddle request: %w", err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("defuddle fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("defuddle returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, defuddleMaxBody))
	if err != nil {
		return "", fmt.Errorf("read defuddle response: %w", err)
	}

	return string(body), nil
}
