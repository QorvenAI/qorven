// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package coworker

import (
	"sync"
	"time"
)

type Note struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Content   string            `json:"content"`
	Tags      []string          `json:"tags,omitempty"`
	Backlinks []string          `json:"backlinks,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	IsLive    bool              `json:"is_live"`
	LiveQuery string            `json:"live_query,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Vault struct {
	Path  string           `json:"path"`
	Notes map[string]*Note `json:"notes"`
	mu    sync.RWMutex
}

type Integration struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config"`
	Active bool              `json:"active"`
}

type MeetingContext struct {
	Title      string    `json:"title"`
	Attendees  []string  `json:"attendees"`
	Time       time.Time `json:"time"`
	PriorNotes []string  `json:"prior_notes"`
	OpenItems  []string  `json:"open_items"`
	Decisions  []string  `json:"decisions"`
}

type EmailDraft struct {
	To      string   `json:"to"`
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Context []string `json:"context"`
}
