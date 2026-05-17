// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// TaskComment is a comment on a task, from a user or agent.
type TaskComment struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	AuthorType string    `json:"author_type"`
	AuthorID   string    `json:"author_id"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaskFile records a file touched by an agent while working a task.
type TaskFile struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Path      string    `json:"path"`
	Operation string    `json:"operation"`
	TouchedAt time.Time `json:"touched_at"`
}

func (gw *Gateway) handleListTaskComments(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []TaskComment{})
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, task_id, author_type, author_id, body, created_at
		 FROM task_comments WHERE task_id = $1 ORDER BY created_at`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	comments := []TaskComment{}
	for rows.Next() {
		var c TaskComment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorType, &c.AuthorID, &c.Body, &c.CreatedAt); err != nil {
			continue
		}
		comments = append(comments, c)
	}
	if comments == nil {
		comments = []TaskComment{}
	}
	writeJSON(w, 200, comments)
}

func (gw *Gateway) handleAddTaskComment(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 503, map[string]string{"error": "database not configured"})
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		writeJSON(w, 400, map[string]string{"error": "body required"})
		return
	}
	var c TaskComment
	err := gw.db.Pool.QueryRow(r.Context(),
		`INSERT INTO task_comments (task_id, author_type, author_id, body)
		 VALUES ($1, 'user', 'user', $2)
		 RETURNING id, task_id, author_type, author_id, body, created_at`,
		id, body.Body).
		Scan(&c.ID, &c.TaskID, &c.AuthorType, &c.AuthorID, &c.Body, &c.CreatedAt)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, c)
}

func (gw *Gateway) handleListTaskFiles(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, 200, []TaskFile{})
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := gw.db.Pool.Query(r.Context(),
		`SELECT id, task_id, path, operation, touched_at
		 FROM task_files WHERE task_id = $1 ORDER BY touched_at DESC`, id)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	files := []TaskFile{}
	for rows.Next() {
		var f TaskFile
		if err := rows.Scan(&f.ID, &f.TaskID, &f.Path, &f.Operation, &f.TouchedAt); err != nil {
			continue
		}
		files = append(files, f)
	}
	if files == nil {
		files = []TaskFile{}
	}
	writeJSON(w, 200, files)
}
