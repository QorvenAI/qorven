// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// SocialComment is a team collaboration comment on a social post.
type SocialComment struct {
	ID         string          `json:"id"`
	PostID     string          `json:"post_id"`
	AuthorID   string          `json:"author_id"`
	AuthorName string          `json:"author_name"`
	Body       string          `json:"body"`
	ParentID   *string         `json:"parent_id,omitempty"`
	Resolved   bool            `json:"resolved"`
	Replies    []SocialComment `json:"replies,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// handleListSocialComments handles GET /v1/social/posts/{id}/comments
func (gw *Gateway) handleListSocialComments(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	postID := chi.URLParam(r, "id")

	// Load all comments for this post (top-level + replies)
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, post_id, author_id, author_name, body, parent_id, resolved, created_at, updated_at
		 FROM social_post_comments WHERE post_id = $1 ORDER BY created_at ASC`, postID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	all := []SocialComment{}
	byID := map[string]*SocialComment{}
	for rows.Next() {
		var c SocialComment
		rows.Scan(&c.ID, &c.PostID, &c.AuthorID, &c.AuthorName, &c.Body,
			&c.ParentID, &c.Resolved, &c.CreatedAt, &c.UpdatedAt)
		all = append(all, c)
	}

	// Build thread tree: top-level comments get their replies attached
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	top := []SocialComment{}
	for i := range all {
		c := &all[i]
		if c.ParentID == nil {
			top = append(top, *c)
		}
	}
	// Attach replies to their parent
	result := make([]SocialComment, 0, len(top))
	for _, t := range top {
		parent := t
		for i := range all {
			c := &all[i]
			if c.ParentID != nil && *c.ParentID == t.ID {
				parent.Replies = append(parent.Replies, *c)
			}
		}
		result = append(result, parent)
	}

	writeJSON(w, 200, result)
}

// handleCreateSocialComment handles POST /v1/social/posts/{id}/comments
func (gw *Gateway) handleCreateSocialComment(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	postID := chi.URLParam(r, "id")

	var body struct {
		Body       string  `json:"body"`
		ParentID   *string `json:"parent_id,omitempty"`
		AuthorName string  `json:"author_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		writeJSON(w, 400, map[string]string{"error": "body required"})
		return
	}

	// Use authenticated user if available, else fall back to provided author_name
	user := userFromContext(r.Context())
	authorID := "anonymous"
	authorName := body.AuthorName
	if user != nil {
		authorID = user.ID
		if authorName == "" {
			authorName = user.DisplayName
		}
	}
	if authorName == "" {
		authorName = "Team member"
	}

	now := time.Now()
	var id string
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO social_post_comments (post_id, author_id, author_name, body, parent_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$6) RETURNING id`,
		postID, authorID, authorName, body.Body, body.ParentID, now).Scan(&id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	c := SocialComment{
		ID:         id,
		PostID:     postID,
		AuthorID:   authorID,
		AuthorName: authorName,
		Body:       body.Body,
		ParentID:   body.ParentID,
		Resolved:   false,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	writeJSON(w, 201, c)
}

// handleDeleteSocialComment handles DELETE /v1/social/posts/{post_id}/comments/{comment_id}
func (gw *Gateway) handleDeleteSocialComment(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	commentID := chi.URLParam(r, "comment_id")
	gw.db.Pool.Exec(r.Context(), `DELETE FROM social_post_comments WHERE id = $1`, commentID)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// handleResolveSocialComment handles PATCH /v1/social/posts/{post_id}/comments/{comment_id}/resolve
func (gw *Gateway) handleResolveSocialComment(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	commentID := chi.URLParam(r, "comment_id")
	var body struct {
		Resolved bool `json:"resolved"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	gw.db.Pool.Exec(r.Context(),
		`UPDATE social_post_comments SET resolved = $1, updated_at = NOW() WHERE id = $2`,
		body.Resolved, commentID)
	writeJSON(w, 200, map[string]string{"status": "updated"})
}
