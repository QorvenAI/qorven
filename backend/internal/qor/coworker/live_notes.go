// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package coworker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// live_notes.go — Auto-updating notes that track topics across platforms.

type LiveNoteEngine struct {
	vault    *Vault
	searchFn func(ctx context.Context, query string) ([]SearchHit, error)
	mu       sync.Mutex
	running  map[string]*liveNoteJob
}

type SearchHit struct {
	Platform string
	Title    string
	URL      string
	Snippet  string
	Score    float64
}

type liveNoteJob struct {
	noteID   string
	query    string
	interval time.Duration
	cancel   context.CancelFunc
}

func NewLiveNoteEngine(vault *Vault, searchFn func(ctx context.Context, query string) ([]SearchHit, error)) *LiveNoteEngine {
	return &LiveNoteEngine{vault: vault, searchFn: searchFn, running: make(map[string]*liveNoteJob)}
}

// Create creates a live note that auto-updates by searching for the query.
func (e *LiveNoteEngine) Create(title, query string, interval time.Duration) (*Note, error) {
	if interval <= 0 { interval = 1 * time.Hour }

	note := &Note{
		ID:        slugify(title),
		Title:     title,
		Content:   fmt.Sprintf("*Live note — updates every %s*\n\nQuery: %s\n\n---\n\nLoading...", interval, query),
		IsLive:    true,
		LiveQuery: query,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  map[string]string{"interval": interval.String()},
	}

	if err := e.vault.Save(note); err != nil { return nil, err }

	// Start background updater
	ctx, cancel := context.WithCancel(context.Background())
	job := &liveNoteJob{noteID: note.ID, query: query, interval: interval, cancel: cancel}

	e.mu.Lock()
	e.running[note.ID] = job
	e.mu.Unlock()

	go e.runUpdater(ctx, job)
	return note, nil
}

// Stop stops a live note from updating.
func (e *LiveNoteEngine) Stop(noteID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if job, ok := e.running[noteID]; ok {
		job.cancel()
		delete(e.running, noteID)
	}
	if note, ok := e.vault.Get(noteID); ok {
		note.IsLive = false
		e.vault.Save(note)
	}
}

// StopAll stops all live notes.
func (e *LiveNoteEngine) StopAll() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, job := range e.running { job.cancel() }
	e.running = make(map[string]*liveNoteJob)
}

func (e *LiveNoteEngine) runUpdater(ctx context.Context, job *liveNoteJob) {
	// Run immediately, then on interval
	e.updateNote(ctx, job)

	ticker := time.NewTicker(job.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done(): return
		case <-ticker.C: e.updateNote(ctx, job)
		}
	}
}

func (e *LiveNoteEngine) updateNote(ctx context.Context, job *liveNoteJob) {
	if e.searchFn == nil { return }

	hits, err := e.searchFn(ctx, job.query)
	if err != nil { return }

	// Build updated content
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*Live note — last updated %s*\n\n", time.Now().Format("Jan 2, 3:04 PM")))
	b.WriteString(fmt.Sprintf("Query: %s\n\n---\n\n", job.query))

	if len(hits) == 0 {
		b.WriteString("No results found.\n")
	} else {
		// Group by platform
		byPlatform := map[string][]SearchHit{}
		for _, h := range hits { byPlatform[h.Platform] = append(byPlatform[h.Platform], h) }

		for platform, items := range byPlatform {
			b.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(platform)))
			for _, h := range items {
				b.WriteString(fmt.Sprintf("- **%s**\n  %s\n  [Link](%s)\n\n", h.Title, h.Snippet, h.URL))
			}
		}
	}

	// Update the note in vault
	note, ok := e.vault.Get(job.noteID)
	if !ok { return }
	note.Content = b.String()
	note.UpdatedAt = time.Now()
	e.vault.Save(note)
}
