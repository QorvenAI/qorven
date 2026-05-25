// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SendFileTool lets an agent deliver a workspace file to the user as a
// download. The file is copied to a temp serve directory, assigned a
// random token, and a download URL is returned. The gateway exposes
// GET /v1/files/download/{token} to serve these files.
type SendFileTool struct {
	workspace string
	onSend    func(token, filename, mime string) // notifies gateway to emit a download notification
}

func NewSendFileTool(workspace string, onSend func(token, filename, mime string)) *SendFileTool {
	return &SendFileTool{workspace: workspace, onSend: onSend}
}

func (t *SendFileTool) Name() string { return "send_file" }
func (t *SendFileTool) Description() string {
	return "Deliver a file to the user as a download. The file must exist in the workspace. " +
		"Returns a download URL the user can click. Use this after creating reports, exports, archives, or any file the user needs."
}
func (t *SendFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file, relative to workspace or absolute within workspace.",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "Download filename shown to the user. Defaults to the file's basename.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *SendFileTool) Execute(ctx context.Context, args map[string]any) *Result {
	path, _ := args["path"].(string)
	if path == "" {
		return ErrorResult("path is required")
	}

	// Resolve to absolute path; restrict to workspace
	if !filepath.IsAbs(path) {
		path = filepath.Join(t.workspace, path)
	}
	path = filepath.Clean(path)

	// Security: must be within workspace, /tmp, or /workspace (sandbox container path)
	wsClean := filepath.Clean(t.workspace)
	if !strings.HasPrefix(path, wsClean) && !strings.HasPrefix(path, "/tmp") && !strings.HasPrefix(path, "/workspace") {
		return ErrorResult("path is outside the workspace")
	}

	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult("file not found: " + err.Error())
	}
	if info.IsDir() {
		return ErrorResult("path is a directory; use a zip tool to archive it first")
	}

	filename, _ := args["filename"].(string)
	if filename == "" {
		filename = filepath.Base(path)
	}

	// Copy to serve directory
	token := uuid.New().String()
	serveDir := "/tmp/qorven-serve"
	os.MkdirAll(serveDir, 0700)
	dest := filepath.Join(serveDir, token)

	if err := copyFile(path, dest); err != nil {
		return ErrorResult("failed to stage file: " + err.Error())
	}

	// Register token globally so the HTTP handler can serve it
	fileServeRegistry.store(token, &servedFile{
		path:     dest,
		filename: filename,
		mime:     guessMIME(filename),
		expires:  time.Now().Add(24 * time.Hour),
	})

	// Notify gateway (emits a notification with download link)
	if t.onSend != nil {
		t.onSend(token, filename, guessMIME(filename))
	}

	return TextResult(fmt.Sprintf("File ready for download: %s\nToken: %s", filename, token))
}

// --- Global serve registry ---

type servedFile struct {
	path     string
	filename string
	mime     string
	expires  time.Time
}

type fileRegistry struct {
	mu    sync.RWMutex
	files map[string]*servedFile
}

var fileServeRegistry = &fileRegistry{files: make(map[string]*servedFile)}

func (r *fileRegistry) store(token string, f *servedFile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[token] = f
	// Prune expired entries
	for k, v := range r.files {
		if time.Now().After(v.expires) {
			delete(r.files, k)
		}
	}
}

func (r *fileRegistry) get(token string) (*servedFile, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.files[token]
	if ok && time.Now().After(f.expires) {
		return nil, false
	}
	return f, ok
}

// GetServedFile returns the metadata for a download token. Used by the HTTP handler.
func GetServedFile(token string) (*ServedFileInfo, bool) {
	f, ok := fileServeRegistry.get(token)
	if !ok {
		return nil, false
	}
	return &ServedFileInfo{Path: f.path, Filename: f.filename, MIME: f.mime}, true
}

// ServedFileInfo is the public view of a staged download.
type ServedFileInfo struct {
	Path     string
	Filename string
	MIME     string
}

// --- helpers ---

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, io.LimitReader(in, 500*1024*1024)) // 500 MiB cap
	return err
}

func guessMIME(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimes := map[string]string{
		".pdf":  "application/pdf",
		".zip":  "application/zip",
		".gz":   "application/gzip",
		".tar":  "application/x-tar",
		".csv":  "text/csv",
		".txt":  "text/plain",
		".md":   "text/markdown",
		".json": "application/json",
		".html": "text/html",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".svg":  "image/svg+xml",
		".mp4":  "video/mp4",
		".mp3":  "audio/mpeg",
	}
	if m, ok := mimes[ext]; ok {
		return m
	}
	return "application/octet-stream"
}
