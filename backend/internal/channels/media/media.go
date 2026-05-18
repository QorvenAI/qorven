// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package media provides shared media utilities for all channel implementations.
package media

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// Media type constants.
const (
	TypeImage     = "image"
	TypeVideo     = "video"
	TypeAudio     = "audio"
	TypeVoice     = "voice"
	TypeDocument  = "document"
	TypeAnimation = "animation"
)

const docMaxChars = 200_000

// MediaInfo contains information about a downloaded media file.
type MediaInfo struct {
	Type        string
	FilePath    string
	FileID      string
	SourceURL   string
	ContentType string
	FileName    string
	FileSize    int64
	Transcript  string
	FromReply   bool
}

// BuildMediaTags generates content tags for media items.
func BuildMediaTags(mediaList []MediaInfo) string {
	var tags []string
	for _, m := range mediaList {
		var tag string
		switch m.Type {
		case TypeImage:
			if m.SourceURL != "" {
				tag = fmt.Sprintf("<media:image url=%q>", m.SourceURL)
			} else {
				tag = "<media:image>"
			}
		case TypeVideo, TypeAnimation:
			tag = "<media:video>"
		case TypeAudio:
			if m.Transcript != "" {
				tag = fmt.Sprintf("<media:audio>\n<transcript>%s</transcript>", html.EscapeString(m.Transcript))
			} else {
				tag = "<media:audio>"
			}
		case TypeVoice:
			if m.Transcript != "" {
				tag = fmt.Sprintf("<media:voice>\n<transcript>%s</transcript>", html.EscapeString(m.Transcript))
			} else {
				tag = "<media:voice>"
			}
		case TypeDocument:
			if m.FileName != "" {
				tag = fmt.Sprintf("<media:document name=%q>", m.FileName)
			} else {
				tag = "<media:document>"
			}
		}
		if tag != "" {
			if m.FromReply {
				tag += " (from replied message)"
			}
			tags = append(tags, tag)
		}
	}
	return strings.Join(tags, "\n")
}

var textExtensions = map[string]string{
	".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
	".json": "application/json", ".yaml": "text/yaml", ".yml": "text/yaml",
	".xml": "text/xml", ".log": "text/plain", ".ini": "text/plain",
	".sh": "text/x-shellscript", ".py": "text/x-python", ".go": "text/x-go",
	".js": "text/javascript", ".ts": "text/typescript", ".html": "text/html",
	".css": "text/css", ".sql": "text/x-sql", ".rs": "text/x-rust",
	".java": "text/x-java", ".c": "text/x-c", ".cpp": "text/x-c++",
	".rb": "text/x-ruby", ".php": "text/x-php", ".toml": "text/x-toml",
}

// ExtractDocumentContent reads a document file and returns its content wrapped in XML tags.
func ExtractDocumentContent(filePath, fileName string) (string, error) {
	if filePath == "" {
		return fmt.Sprintf("[File: %s — download failed]", fileName), nil
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	mime, isText := textExtensions[ext]
	if !isText {
		return fmt.Sprintf("[File: %s — use read_document tool to analyze this file]", fileName), nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", fileName, err)
	}

	content := string(data)
	if len(content) > docMaxChars {
		content = content[:docMaxChars] + "\n... [truncated]"
	}

	escaped := html.EscapeString(content)
	return fmt.Sprintf("<file name=%q mime=%q>\n%s\n</file>", fileName, mime, escaped), nil
}

// DetectMimeType returns the MIME type for a file extension.
func DetectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	case ".opus":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".m4a":
		return "audio/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		if mime, ok := textExtensions[ext]; ok {
			return mime
		}
		return "application/octet-stream"
	}
}

// MediaKindFromMime returns the media kind based on MIME type prefix.
func MediaKindFromMime(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return TypeImage
	case strings.HasPrefix(mimeType, "video/"):
		return TypeVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return TypeAudio
	default:
		return TypeDocument
	}
}
