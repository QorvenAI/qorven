// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultSTTTimeout     = 30
	sttTranscribeEndpoint = "/transcribe_audio"
)

var (
	sttClient     *http.Client
	sttClientOnce sync.Once
	sttSem        = make(chan struct{}, 4) // max 4 concurrent STT calls
)

func getSTTClient() *http.Client {
	sttClientOnce.Do(func() {
		sttClient = &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	})
	return sttClient
}

// STTConfig holds configuration for the Speech-to-Text proxy service.
type STTConfig struct {
	ProxyURL       string
	APIKey         string
	TenantID       string
	TimeoutSeconds int
}

type sttResponse struct {
	Transcript string `json:"transcript"`
}

// TranscribeAudio calls the configured STT proxy service with the given audio file.
// Returns ("", nil) when cfg.ProxyURL or filePath is empty.
func TranscribeAudio(ctx context.Context, cfg STTConfig, filePath string) (string, error) {
	if cfg.ProxyURL == "" || filePath == "" {
		return "", nil
	}

	timeoutSec := cfg.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = DefaultSTTTimeout
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("stt: open audio file %q: %w", filePath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	fw, err := w.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return "", fmt.Errorf("stt: create form file field: %w", err)
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", fmt.Errorf("stt: write audio bytes: %w", err)
	}

	if cfg.TenantID != "" {
		w.WriteField("tenant_id", cfg.TenantID)
	}
	w.Close()

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	url := cfg.ProxyURL + sttTranscribeEndpoint
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, &body)
	if err != nil {
		return "", fmt.Errorf("stt: build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	slog.Debug("stt: calling proxy", "url", url, "file", filepath.Base(filePath))

	select {
	case sttSem <- struct{}{}:
		defer func() { <-sttSem }()
	case <-reqCtx.Done():
		return "", fmt.Errorf("stt: context cancelled waiting for slot: %w", reqCtx.Err())
	}

	resp, err := getSTTClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("stt: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("stt: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt: upstream returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result sttResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("stt: parse response: %w", err)
	}

	slog.Debug("stt: transcript received", "length", len(result.Transcript))
	return result.Transcript, nil
}
