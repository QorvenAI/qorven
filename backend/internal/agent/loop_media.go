// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"path/filepath"
	"strings"
)

// MediaResult represents a media file produced by a tool.
type MediaResult struct {
	Path        string
	ContentType string
	AsVoice     bool // for TTS voice messages
}

// ParseMediaResult extracts a MediaResult from a tool result string containing "MEDIA:" prefix.
// Handles formats: "MEDIA:/path/to/file" and "[[audio_as_voice]]\nMEDIA:/path/to/file".
// Returns nil if no MEDIA: prefix is found.
func ParseMediaResult(toolOutput string) *MediaResult {
	s := toolOutput
	asVoice := false

	// Check for [[audio_as_voice]] tag (TTS voice messages)
	if strings.Contains(s, "[[audio_as_voice]]") {
		asVoice = true
		s = strings.ReplaceAll(s, "[[audio_as_voice]]", "")
	}

	s = strings.TrimSpace(s)

	// Only match MEDIA: at the beginning of the string
	if !strings.HasPrefix(s, "MEDIA:") {
		return nil
	}
	path := strings.TrimSpace(s[6:])
	if path == "" {
		return nil
	}
	// Take only the first line
	if nl := strings.IndexByte(path, '\n'); nl >= 0 {
		path = strings.TrimSpace(path[:nl])
	}

	return &MediaResult{
		Path:        path,
		ContentType: MimeFromExt(filepath.Ext(path)),
		AsVoice:     asVoice,
	}
}

// DeduplicateMedia removes duplicate media results by path.
func DeduplicateMedia(media []MediaResult) []MediaResult {
	if len(media) <= 1 {
		return media
	}
	seen := make(map[string]bool, len(media))
	result := make([]MediaResult, 0, len(media))
	for _, m := range media {
		if seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		result = append(result, m)
	}
	return result
}

// MimeFromExt returns a MIME type for common media file extensions.
func MimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".txt":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".zip":
		return "application/zip"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// IsImageMime returns true if the MIME type is an image.
func IsImageMime(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

// IsAudioMime returns true if the MIME type is audio.
func IsAudioMime(mime string) bool {
	return strings.HasPrefix(mime, "audio/")
}

// IsVideoMime returns true if the MIME type is video.
func IsVideoMime(mime string) bool {
	return strings.HasPrefix(mime, "video/")
}

// IsDocumentMime returns true if the MIME type is a document.
func IsDocumentMime(mime string) bool {
	switch mime {
	case "application/pdf", "text/plain", "text/csv", "text/markdown",
		"application/msword", "application/vnd.ms-excel",
		"application/json", "text/html", "application/xml":
		return true
	}
	return false
}
