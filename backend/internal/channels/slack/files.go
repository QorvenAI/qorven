// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package slack

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"
)

// File handling: download with auth safety, upload, SSRF protection.

const maxFileBytes = 20 * 1024 * 1024 // 20MB

// Allowed Slack CDN hosts for file downloads (SSRF protection)
var allowedSlackHosts = map[string]bool{
	"files.slack.com":       true,
	"files-pri.slack.com":   true,
	"files-tmb.slack.com":   true,
	"avatars.slack-edge.com": true,
}

// downloadSlackFile downloads a file from Slack with auth, following redirects safely.
// Only sends Authorization header to Slack hosts (prevents credential leakage on cross-origin redirects).
func (s *SlackChannel) downloadSlackFile(name, urlPrivate string) (string, error) {
	if urlPrivate == "" { return "", fmt.Errorf("no URL") }

	// Validate host
	parsed, err := url.Parse(urlPrivate)
	if err != nil { return "", err }
	if !isAllowedSlackHost(parsed.Host) {
		return "", fmt.Errorf("blocked: host %s not in Slack CDN allowlist", parsed.Host)
	}

	// Create temp file
	os.MkdirAll("/tmp/qorven-slack", 0755)
	ext := filepath.Ext(name)
	if ext == "" { ext = ".bin" }
	tmpFile, err := os.CreateTemp("/tmp/qorven-slack", "slack-*"+ext)
	if err != nil { return "", err }
	defer tmpFile.Close()

	// Download with manual redirect following (same-host auth only)
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Only forward auth to Slack hosts
			if !isAllowedSlackHost(req.URL.Host) {
				req.Header.Del("Authorization")
			}
			if len(via) >= 5 { return fmt.Errorf("too many redirects") }
			return nil
		},
	}

	req, _ := http.NewRequest("GET", urlPrivate, nil)
	req.Header.Set("Authorization", "Bearer "+s.cfg.BotToken)
	resp, err := client.Do(req)
	if err != nil { os.Remove(tmpFile.Name()); return "", err }
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxFileBytes))
	if err != nil { os.Remove(tmpFile.Name()); return "", err }

	slog.Info("slack.file.downloaded", "name", name, "bytes", written)
	return tmpFile.Name(), nil
}

// uploadSlackFile uploads a file to a Slack channel/thread
func (s *SlackChannel) uploadSlackFile(channelID, threadTS, filename, content string) error {
	params := slackapi.UploadFileParameters{
		Channel:  channelID,
		Filename: filename,
		Content:  content,
	}
	if threadTS != "" {
		params.ThreadTimestamp = threadTS
	}
	_, err := s.api.UploadFile(params)
	return err
}

func isAllowedSlackHost(host string) bool {
	if allowedSlackHosts[host] { return true }
	// Also allow *.slack.com subdomains
	return strings.HasSuffix(host, ".slack.com")
}

// resolveSlackMedia extracts file info from Slack message files
type slackMediaItem struct {
	Name     string
	MimeType string
	Size     int
	URL      string
	Content  string // extracted text content (for text files)
}

func (s *SlackChannel) resolveSlackMedia(files []slackapi.File) ([]slackMediaItem, string) {
	var items []slackMediaItem
	var extraContent string

	for _, f := range files {
		item := slackMediaItem{
			Name:     f.Name,
			MimeType: f.Mimetype,
			Size:     f.Size,
			URL:      f.URLPrivateDownload,
		}

		// For small text files, try to get content inline
		if isSlackTextFile(f.Mimetype) && f.Size < 50000 {
			if localPath, err := s.downloadSlackFile(f.Name, f.URLPrivateDownload); err == nil {
				if data, err := os.ReadFile(localPath); err == nil {
					item.Content = string(data)
				}
				os.Remove(localPath)
			}
		}

		items = append(items, item)
		extraContent += buildSlackMediaTag(item)
	}
	return items, extraContent
}

func buildSlackMediaTag(item slackMediaItem) string {
	if item.Content != "" {
		return fmt.Sprintf("\n<attached_file name=\"%s\" type=\"%s\">\n%s\n</attached_file>", item.Name, item.MimeType, item.Content)
	}
	return fmt.Sprintf("\n[File: %s (%s, %d bytes)]", item.Name, item.MimeType, item.Size)
}

func isSlackTextFile(mime string) bool {
	for _, prefix := range []string{"text/", "application/json", "application/xml", "application/javascript"} {
		if strings.HasPrefix(mime, prefix) { return true }
	}
	return false
}
