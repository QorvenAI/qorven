// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qorvenai/qorven/internal/config"
)

// SocialMediaAsset is a media file stored in the social media library.
type SocialMediaAsset struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	Name         string    `json:"name"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	Width        *int      `json:"width,omitempty"`
	Height       *int      `json:"height,omitempty"`
	AltText      string    `json:"alt_text"`
	Tags         []string  `json:"tags"`
	URL          string    `json:"url"`
	CreatedAt    time.Time `json:"created_at"`
}

// socialMediaDir returns the persistent directory for social media assets for an agent.
func socialMediaDir(agentID string) string {
	return config.Sub("social-media", agentID)
}

// handleSocialMediaUpload handles POST /v1/social/media — multipart upload to media library.
func (gw *Gateway) handleSocialMediaUpload(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50MB max for media
		writeJSON(w, 400, map[string]string{"error": "file too large (max 50MB)"})
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
		writeJSON(w, 400, map[string]string{"error": "agent_id required"})
		return
	}
	altText := r.FormValue("alt_text")

	// Validate MIME type — only images and videos for social media
	allowedMIME := map[string]bool{
		"image/jpeg": true, "image/png": true, "image/gif": true,
		"image/webp": true, "image/svg+xml": true,
		"video/mp4": true, "video/quicktime": true, "video/webm": true,
		"video/x-msvideo": true, "video/mpeg": true,
	}
	ct := header.Header.Get("Content-Type")
	// If Content-Type not set in part header, detect from extension
	if ct == "" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		extMIME := map[string]string{
			".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png",
			".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml",
			".mp4": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm",
			".avi": "video/x-msvideo", ".mpeg": "video/mpeg", ".mpg": "video/mpeg",
		}
		ct = extMIME[ext]
	}
	if ct == "" || !allowedMIME[ct] {
		writeJSON(w, 400, map[string]string{"error": "unsupported file type — only images and videos allowed"})
		return
	}

	// Generate file ID and save to disk
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	fileID := hex.EncodeToString(idBytes)

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		// Derive extension from MIME
		mimeToExt := map[string]string{
			"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif",
			"image/webp": ".webp", "image/svg+xml": ".svg",
			"video/mp4": ".mp4", "video/quicktime": ".mov", "video/webm": ".webm",
		}
		ext = mimeToExt[ct]
	}

	dir := socialMediaDir(agentID)
	os.MkdirAll(dir, 0755)
	savePath := filepath.Join(dir, fileID+ext)

	dst, err := os.Create(savePath)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": "failed to save file"})
		return
	}
	written, err := io.Copy(dst, io.LimitReader(file, 50<<20))
	dst.Close()
	if err != nil {
		os.Remove(savePath)
		writeJSON(w, 500, map[string]string{"error": "failed to write file"})
		return
	}

	// Store in DB
	var id string
	err = gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO social_media_assets (agent_id, name, original_name, path, mime_type, size, alt_text, tags, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'{}'::text[],$8) RETURNING id`,
		agentID, fileID+ext, header.Filename, savePath, ct, written, altText, time.Now(),
	).Scan(&id)
	if err != nil {
		os.Remove(savePath)
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	asset := SocialMediaAsset{
		ID:           id,
		AgentID:      agentID,
		Name:         fileID + ext,
		OriginalName: header.Filename,
		MimeType:     ct,
		Size:         written,
		AltText:      altText,
		Tags:         []string{},
		URL:          "/api/v1/social/media/" + id + "/content",
	}
	writeJSON(w, 201, asset)
}

// handleListSocialMedia handles GET /v1/social/media — paginated, searchable list.
func (gw *Gateway) handleListSocialMedia(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	search := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	mediaType := r.URL.Query().Get("type") // "image" or "video"

	limit := 50
	if v, _ := strconv.Atoi(limitStr); v > 0 && v <= 200 {
		limit = v
	}
	offset := 0
	if v, _ := strconv.Atoi(offsetStr); v >= 0 {
		offset = v
	}

	query := `SELECT id, agent_id, name, original_name, path, mime_type, size,
	           width, height, alt_text, tags, created_at
	           FROM social_media_assets WHERE 1=1`
	args := []any{}

	if agentID != "" {
		args = append(args, agentID)
		query += ` AND agent_id = $` + strconv.Itoa(len(args))
	}
	if search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		query += ` AND (LOWER(original_name) LIKE $` + strconv.Itoa(len(args)) +
			` OR LOWER(alt_text) LIKE $` + strconv.Itoa(len(args)) + `)`
	}
	if mediaType == "image" {
		query += ` AND mime_type LIKE 'image/%'`
	} else if mediaType == "video" {
		query += ` AND mime_type LIKE 'video/%'`
	}

	// Count
	countQuery := strings.Replace(query, `SELECT id, agent_id, name, original_name, path, mime_type, size,
	           width, height, alt_text, tags, created_at`, `SELECT COUNT(*)`, 1)
	var total int
	gw.db.Pool.QueryRow(r.Context(), countQuery, args...).Scan(&total)

	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	rows, err := gw.db.Pool.Query(r.Context(), query, args...)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	assets := []SocialMediaAsset{}
	for rows.Next() {
		var a SocialMediaAsset
		var diskPath string
		var tags []string
		rows.Scan(&a.ID, &a.AgentID, &a.Name, &a.OriginalName, &diskPath, &a.MimeType,
			&a.Size, &a.Width, &a.Height, &a.AltText, &tags, &a.CreatedAt)
		a.Tags = tags
		if a.Tags == nil {
			a.Tags = []string{}
		}
		a.URL = "/api/v1/social/media/" + a.ID + "/content"
		assets = append(assets, a)
	}

	writeJSON(w, 200, map[string]any{
		"assets": assets,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleGetSocialMediaAsset handles GET /v1/social/media/{id}.
func (gw *Gateway) handleGetSocialMediaAsset(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")
	var a SocialMediaAsset
	var path string
	var tags []string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT id, agent_id, name, original_name, path, mime_type, size, width, height, alt_text, tags, created_at
		 FROM social_media_assets WHERE id = $1`, id).Scan(
		&a.ID, &a.AgentID, &a.Name, &a.OriginalName, &path, &a.MimeType,
		&a.Size, &a.Width, &a.Height, &a.AltText, &tags, &a.CreatedAt)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	a.Tags = tags
	if a.Tags == nil {
		a.Tags = []string{}
	}
	a.URL = "/api/v1/social/media/" + a.ID + "/content"
	writeJSON(w, 200, a)
}

// handleSocialMediaContent handles GET /v1/social/media/{id}/content — serves file bytes.
func (gw *Gateway) handleSocialMediaContent(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")

	var path, mimeType string
	err := gw.db.Pool.QueryRow(r.Context(),
		`SELECT path, mime_type FROM social_media_assets WHERE id = $1`, id).Scan(&path, &mimeType)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	io.Copy(w, f)
}

// handleDeleteSocialMedia handles DELETE /v1/social/media/{id}.
func (gw *Gateway) handleDeleteSocialMedia(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")

	// Get path for disk cleanup
	var path string
	gw.db.Pool.QueryRow(r.Context(),
		`SELECT path FROM social_media_assets WHERE id = $1`, id).Scan(&path)

	_, err := gw.db.Pool.Exec(r.Context(),
		`DELETE FROM social_media_assets WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	if path != "" {
		os.Remove(path)
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleUpdateSocialMediaAsset handles PATCH /v1/social/media/{id} — update alt_text/tags.
func (gw *Gateway) handleUpdateSocialMediaAsset(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		AltText *string  `json:"alt_text"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}

	if body.AltText != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE social_media_assets SET alt_text = $1 WHERE id = $2`, *body.AltText, id)
	}
	if body.Tags != nil {
		gw.db.Pool.Exec(r.Context(),
			`UPDATE social_media_assets SET tags = $1 WHERE id = $2`, body.Tags, id)
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}
