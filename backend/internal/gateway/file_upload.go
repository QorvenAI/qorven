// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileUploadResponse is returned after a successful file upload.
type FileUploadResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	ContentType   string `json:"content_type"`
	ExtractedText string `json:"extracted_text,omitempty"`
	Path          string `json:"path"`
}

// handleFileUpload handles POST /v1/files — uploads to Soul's workspace.
// Mentor: store in workspace/souls/{agent_id}/attachments/{message_id}/
func (gw *Gateway) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil { // 20MB max
		writeJSON(w, 400, map[string]string{"error": "file too large (max 20MB)"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "no file provided"})
		return
	}
	defer file.Close()

	agentID := r.FormValue("agent_id")
	if agentID == "" {
		agentID = "shared"
	}

	// Generate file ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	fileID := hex.EncodeToString(idBytes)

	// Create directory: workspace/souls/{agent_id}/attachments/
	baseDir := filepath.Join("/tmp/qorven-workspace", "souls", agentID, "attachments")
	os.MkdirAll(baseDir, 0755)

	// Save file — whitelist allowed extensions to prevent RCE via uploaded scripts
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowedExts := map[string]bool{
		".txt": true, ".md": true, ".csv": true, ".json": true, ".xml": true, ".yaml": true, ".yml": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".webp": true,
		".zip": true, ".tar": true, ".gz": true,
		".go": true, ".py": true, ".js": true, ".ts": true, ".rs": true, ".java": true, ".c": true, ".cpp": true, ".h": true,
		".html": true, ".css": true, ".sql": true, ".sh": true, ".toml": true, ".ini": true, ".log": true,
	}
	if ext != "" && !allowedExts[ext] {
		ext = ".bin" // neutralize unknown/dangerous extensions
	}
	savePath := filepath.Join(baseDir, fileID+ext)
	dst, err := os.Create(savePath)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save file"})
		return
	}
	defer dst.Close()

	size, err := io.Copy(dst, file)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to write file"})
		return
	}

	// Extract text for text-based files (mentor: extraction at upload time)
	var extractedText string
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = detectContentType(header.Filename)
	}

	if isTextFile(contentType, header.Filename) {
		dst.Seek(0, 0)
		textBytes, err := io.ReadAll(io.LimitReader(dst, 100000)) // 100KB max text
		if err == nil {
			extractedText = string(textBytes)
		}
	}

	// Store metadata in DB
	if gw.db != nil && gw.db.Pool != nil {
		gw.db.Pool.Exec(r.Context(),
			`INSERT INTO drive_files (id, tenant_id, agent_id, name, path, content_type, size, extracted_text, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO NOTHING`,
			fileID, defaultTenant, agentID, header.Filename, savePath, contentType, size, extractedText, time.Now())
		// Queue enrichment asynchronously after text extraction
		if extractedText != "" {
			go gw.enrichDriveFile(context.Background(), fileID)
		}
	}

	slog.Info("file.uploaded", "id", fileID, "name", header.Filename, "size", size, "agent", agentID)

	writeJSON(w, 200, FileUploadResponse{
		ID:            fileID,
		Name:          header.Filename,
		Size:          size,
		ContentType:   contentType,
		ExtractedText: truncateStr(extractedText, 500),
		Path:          savePath,
	})
}

// handleFileContent serves file content by ID.
func (gw *Gateway) handleFileContent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	fileID := r.PathValue("id")
	if fileID == "" {
		writeJSON(w, 400, map[string]string{"error": "file ID required"})
		return
	}

	var path, contentType, name string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT path, content_type, name FROM drive_files WHERE id = $1`, fileID).Scan(&path, &contentType, &name)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "file not found"})
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
	http.ServeFile(w, r, path)
}

func detectContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	types := map[string]string{
		".txt": "text/plain", ".md": "text/markdown", ".csv": "text/csv",
		".json": "application/json", ".xml": "application/xml",
		".py": "text/x-python", ".go": "text/x-go", ".js": "text/javascript",
		".ts": "text/typescript", ".html": "text/html", ".css": "text/css",
		".pdf": "application/pdf", ".png": "image/png", ".jpg": "image/jpeg",
		".jpeg": "image/jpeg", ".gif": "image/gif", ".svg": "image/svg+xml",
		".zip": "application/zip", ".yaml": "text/yaml", ".yml": "text/yaml",
		".rs": "text/x-rust", ".java": "text/x-java", ".rb": "text/x-ruby",
		".sh": "text/x-shellscript", ".sql": "text/x-sql",
	}
	if ct, ok := types[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

func isTextFile(contentType, filename string) bool {
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(filename))
	textExts := map[string]bool{
		".txt": true, ".md": true, ".csv": true, ".json": true, ".xml": true,
		".py": true, ".go": true, ".js": true, ".ts": true, ".html": true,
		".css": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true,
		".rs": true, ".java": true, ".rb": true, ".sh": true, ".sql": true,
		".tsx": true, ".jsx": true, ".vue": true, ".svelte": true,
	}
	return textExts[ext]
}

func truncateStr(s string, n int) string {
	if len(s) <= n { return s }
	return s[:n] + "..."
}
