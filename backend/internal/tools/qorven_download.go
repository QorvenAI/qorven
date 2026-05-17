// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QorvenDownload downloads files from URLs with progress tracking.
type QorvenDownload struct {
	workspace string
}

func NewQorvenDownload(workspace string) *QorvenDownload {
	return &QorvenDownload{workspace: workspace}
}

func (t *QorvenDownload) Name() string { return "qorven_download" }
func (t *QorvenDownload) Description() string {
	return "Download files from URLs to the workspace. Supports HTTP/HTTPS with progress tracking."
}
func (t *QorvenDownload) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":      map[string]any{"type": "string", "description": "URL to download"},
			"filename": map[string]any{"type": "string", "description": "Save as filename (optional, auto-detected from URL)"},
		},
		"required": []string{"url"},
	}
}

func (t *QorvenDownload) Execute(ctx context.Context, args map[string]any) *Result {
	url, _ := args["url"].(string)
	filename, _ := args["filename"].(string)
	// Security: only allow http/https schemes
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ErrorResult("only http:// and https:// URLs supported")
	}
	if url == "" {
		return ErrorResult("url is required")
	}

	if filename == "" {
		parts := strings.Split(url, "/")
		filename = parts[len(parts)-1]
		if filename == "" || len(filename) > 100 {
			filename = "download"
		}
		// Clean query params
		if idx := strings.Index(filename, "?"); idx > 0 {
			filename = filename[:idx]
		}
	}

	destDir := t.workspace
	if destDir == "" {
		destDir = os.TempDir()
	}
	dest := filepath.Join(destDir, filename)
	os.MkdirAll(filepath.Dir(dest), 0755)

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return ErrorResult("invalid URL: " + err.Error())
	}
	req.Header.Set("User-Agent", "Qorven/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ErrorResult("download failed: " + err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ErrorResult(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status))
	}

	f, err := os.Create(dest)
	if err != nil {
		return ErrorResult("create file: " + err.Error())
	}
	defer f.Close()

	written, err := io.Copy(f, io.LimitReader(resp.Body, 100*1024*1024)) // 100MB limit
	if err != nil {
		return ErrorResult("write failed: " + err.Error())
	}

	elapsed := time.Since(start)
	slog.Info("download.complete", "url", url, "file", dest, "bytes", written, "elapsed", elapsed)

	return TextResult(fmt.Sprintf("Downloaded: %s\nSize: %s\nSaved to: %s\nTime: %s",
		filename, formatBytes(written), dest, elapsed.Round(time.Millisecond)))
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}
