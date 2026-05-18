// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package telegram

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

	"github.com/go-telegram/bot"
)

// Full media pipeline: download, process, extract text, build context tags.

const (
	maxMediaBytes    = 20 * 1024 * 1024 // 20MB download limit
	downloadTimeout  = 60 * time.Second
	tempMediaDir     = "/tmp/qorven-media"
)

type MediaInfo struct {
	Type        string // photo, voice, audio, document, video, sticker, video_note
	FileID      string
	FileName    string
	MimeType    string
	FileSize    int
	LocalPath   string // downloaded temp file path
	TextContent string // extracted text (for documents)
	Caption     string
}

type MediaError struct {
	Type    string
	Message string
}

// resolveAllMedia extracts ALL media from a Telegram message
func (t *TelegramChannel) resolveAllMedia(ctx context.Context, msg interface{ /* models.Message fields */ }) []MediaInfo {
	// This is called from the handler with the raw message
	return nil // Implemented inline in handler for type safety
}

// downloadMediaFile downloads a Telegram file to a temp path with progress tracking
func (t *TelegramChannel) downloadMediaFile(ctx context.Context, fileID string) (string, string, error) {
	file, err := t.b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil { return "", "", fmt.Errorf("get file: %w", err) }

	fileURL := t.b.FileDownloadLink(file)
	mimeType := guessMimeType(file.FilePath)

	// Create temp directory
	os.MkdirAll(tempMediaDir, 0755)
	ext := filepath.Ext(file.FilePath)
	if ext == "" { ext = ".bin" }
	tmpFile, err := os.CreateTemp(tempMediaDir, "tg-*"+ext)
	if err != nil { return "", mimeType, err }
	defer tmpFile.Close()

	// Download with timeout and size limit
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(fileURL)
	if err != nil { os.Remove(tmpFile.Name()); return "", mimeType, err }
	defer resp.Body.Close()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxMediaBytes))
	if err != nil { os.Remove(tmpFile.Name()); return "", mimeType, err }

	slog.Info("telegram.media.downloaded", "file", file.FilePath, "bytes", written, "mime", mimeType)
	return tmpFile.Name(), mimeType, nil
}

// buildMediaContextTags creates XML tags for the LLM to understand media
func buildMediaContextTags(media []MediaInfo) string {
	if len(media) == 0 { return "" }
	var sb strings.Builder
	for _, m := range media {
		switch m.Type {
		case "photo":
			sb.WriteString(fmt.Sprintf("<attached_file name=\"%s\" type=\"%s\">\n[Image: %d bytes — describe what you see if asked]\n</attached_file>\n",
				m.FileName, m.MimeType, m.FileSize))
		case "voice", "audio":
			if m.TextContent != "" {
				sb.WriteString(fmt.Sprintf("<voice_transcript name=\"%s\" mime=\"%s\">\n%s\n</voice_transcript>\n",
					m.FileName, m.MimeType, m.TextContent))
			} else {
				sb.WriteString(fmt.Sprintf("[Audio: %s (%s, %d bytes) — transcription not available]\n",
					m.FileName, m.MimeType, m.FileSize))
			}
		case "document":
			if m.TextContent != "" {
				sb.WriteString(fmt.Sprintf("<attached_file name=\"%s\" type=\"%s\">\n%s\n</attached_file>\n",
					m.FileName, m.MimeType, m.TextContent))
			} else {
				sb.WriteString(fmt.Sprintf("[Document: %s (%s, %d bytes)]\n", m.FileName, m.MimeType, m.FileSize))
			}
		case "video":
			sb.WriteString(fmt.Sprintf("[Video: %s (%s, %d bytes)]\n", m.FileName, m.MimeType, m.FileSize))
		case "sticker":
			sb.WriteString(fmt.Sprintf("[Sticker: %s]\n", m.Caption))
		default:
			sb.WriteString(fmt.Sprintf("[%s: %s (%d bytes)]\n", m.Type, m.FileName, m.FileSize))
		}
	}
	return sb.String()
}

// extractDocumentText tries to extract text from common document formats
func extractDocumentText(localPath, mimeType string) string {
	// Read text-based files directly
	if isTextMime(mimeType) {
		data, err := os.ReadFile(localPath)
		if err != nil { return "" }
		text := string(data)
		if len(text) > 50000 { text = text[:50000] + "\n[truncated — 50KB limit]" }
		return text
	}
	// For PDF/DOCX — would need external tools (pdftotext, pandoc)
	// Return empty for now — can be wired to extraction service later
	return ""
}

func isTextMime(mime string) bool {
	textMimes := []string{"text/", "application/json", "application/xml", "application/javascript",
		"application/x-yaml", "application/toml", "application/csv"}
	for _, prefix := range textMimes {
		if strings.HasPrefix(mime, prefix) { return true }
	}
	return false
}

func isTextFile(path string) bool {
	textExts := []string{".txt", ".md", ".json", ".csv", ".xml", ".yaml", ".yml", ".toml", ".log",
		".py", ".go", ".js", ".ts", ".rs", ".java", ".c", ".cpp", ".h", ".sh", ".sql", ".html", ".css"}
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range textExts {
		if ext == e { return true }
	}
	return false
}

func guessMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	mimeMap := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
		".mp3": "audio/mpeg", ".ogg": "audio/ogg", ".wav": "audio/wav", ".m4a": "audio/mp4",
		".mp4": "video/mp4", ".avi": "video/avi", ".mkv": "video/x-matroska",
		".pdf": "application/pdf", ".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".txt": "text/plain", ".md": "text/markdown", ".json": "application/json", ".csv": "text/csv",
		".py": "text/x-python", ".go": "text/x-go", ".js": "text/javascript",
	}
	if mime, ok := mimeMap[ext]; ok { return mime }
	return "application/octet-stream"
}

// cleanupTempMedia removes downloaded temp files older than 1 hour
func cleanupTempMedia() {
	entries, err := os.ReadDir(tempMediaDir)
	if err != nil { return }
	cutoff := time.Now().Add(-1 * time.Hour)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil { continue }
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(tempMediaDir, e.Name()))
		}
	}
}
