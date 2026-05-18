// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package agent

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/providers"
)

// maxImageBytes is the safety limit for reading image files (10MB).
const maxImageBytes = 10 * 1024 * 1024

// MediaFile represents an input media file.
type MediaFile struct {
	Path     string
	MimeType string
}

// MediaRef is a lightweight reference to a persisted media file.
type MediaRef struct {
	ID       string
	MimeType string
	Kind     string // "image", "document", "audio", "video"
	Path     string
}

// LoadImages reads local image files and returns base64-encoded ImageContent slices.
// Non-image files and files that fail to read are skipped with a warning log.
func LoadImages(files []MediaFile) []providers.ImageContent {
	if len(files) == 0 {
		return nil
	}

	var images []providers.ImageContent
	for _, f := range files {
		mime := f.MimeType
		if mime == "" {
			mime = inferImageMime(f.Path)
		}
		if !strings.HasPrefix(mime, "image/") {
			continue
		}

		data, err := os.ReadFile(f.Path)
		if err != nil {
			slog.Warn("vision: failed to read image file", "path", f.Path, "error", err)
			continue
		}
		if len(data) > maxImageBytes {
			slog.Warn("vision: image file too large, skipping", "path", f.Path, "size", len(data))
			continue
		}

		images = append(images, providers.ImageContent{
			MimeType: mime,
			Data:     base64.StdEncoding.EncodeToString(data),
		})
	}
	return images
}

// PersistMedia saves media files to the workspace .uploads/ directory.
// Returns lightweight MediaRefs with persisted paths.
func PersistMedia(files []MediaFile, workspace string) []MediaRef {
	if workspace == "" {
		slog.Warn("media: no workspace, cannot persist media")
		return nil
	}

	uploadsDir := filepath.Join(workspace, ".uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		slog.Warn("media: failed to create .uploads dir", "dir", uploadsDir, "error", err)
		return nil
	}

	// Verify .uploads is a real directory (not symlink) to prevent attacks.
	if fi, err := os.Lstat(uploadsDir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		slog.Warn("media: .uploads is a symlink, refusing to use", "dir", uploadsDir)
		return nil
	}

	var refs []MediaRef
	for _, f := range files {
		mime := f.MimeType
		if mime == "" {
			mime = mimeFromExtMedia(filepath.Ext(f.Path))
		}
		kind := mediaKindFromMime(mime)

		id := uuid.New().String()
		ext := extFromMime(mime)
		if ext == "" {
			ext = filepath.Ext(f.Path)
		}
		dstPath := filepath.Join(uploadsDir, id+ext)

		if err := copyMediaFile(f.Path, dstPath); err != nil {
			slog.Warn("media: failed to persist file", "path", f.Path, "error", err)
			continue
		}

		refs = append(refs, MediaRef{
			ID:       id,
			MimeType: mime,
			Kind:     kind,
			Path:     dstPath,
		})
		slog.Debug("media: persisted file", "id", id, "kind", kind, "path", dstPath)
	}
	return refs
}

// copyMediaFile copies src to dst using buffered I/O.
func copyMediaFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// EnrichDocumentPaths updates the last user message to include persisted file paths.
func EnrichDocumentPaths(messages []providers.Message, refs []MediaRef) {
	if len(messages) == 0 {
		return
	}
	lastIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return
	}

	content := messages[lastIdx].Content
	for _, ref := range refs {
		if ref.Kind != "document" || ref.Path == "" {
			continue
		}
		// Append document path info to message
		content += fmt.Sprintf("\n[Attached document: %s (path: %s)]", ref.ID, ref.Path)
	}
	messages[lastIdx].Content = content
}

// EnrichImageIDs updates the last user message to embed persisted media IDs.
func EnrichImageIDs(messages []providers.Message, refs []MediaRef) {
	if len(messages) == 0 {
		return
	}
	lastIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastIdx = i
			break
		}
	}
	if lastIdx < 0 {
		return
	}

	content := messages[lastIdx].Content
	for _, ref := range refs {
		if ref.Kind != "image" || ref.Path == "" {
			continue
		}
		content += fmt.Sprintf("\n[Attached image: %s (path: %s)]", ref.ID, ref.Path)
	}
	messages[lastIdx].Content = content
}

// inferImageMime infers MIME type from file extension.
func inferImageMime(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// mimeFromExtMedia returns MIME type from extension.
func mimeFromExtMedia(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".doc", ".docx":
		return "application/msword"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// mediaKindFromMime returns the media kind from MIME type.
func mediaKindFromMime(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	default:
		return "document"
	}
}

// extFromMime returns file extension from MIME type.
func extFromMime(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "audio/mpeg":
		return ".mp3"
	case "audio/wav":
		return ".wav"
	case "audio/ogg":
		return ".ogg"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	default:
		return ""
	}
}
