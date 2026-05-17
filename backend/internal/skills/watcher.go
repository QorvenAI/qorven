// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package skills

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounce = 500 * time.Millisecond

// Watcher monitors skill directories for SKILL.md changes and bumps the loader version.
type Watcher struct {
	loader *Loader
	fsw    *fsnotify.Watcher
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	timer  *time.Timer
	pending bool
}

// NewWatcher creates a skills directory watcher.
func NewWatcher(loader *Loader) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{loader: loader, fsw: fsw}, nil
}

// Start begins watching all skill directories for changes.
func (w *Watcher) Start(ctx context.Context) error {
	dirs := w.loader.Dirs()
	watched := 0

	for _, dir := range dirs {
		if err := w.fsw.Add(dir); err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("skills.watcher: cannot watch", "path", dir, "error", err)
			}
			continue
		}
		watched++

		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				if err := w.fsw.Add(filepath.Join(dir, e.Name())); err == nil {
					watched++
				}
			}
		}
	}

	ctx, w.cancel = context.WithCancel(ctx)
	w.wg.Add(1)
	go w.loop(ctx)

	slog.Info("skills watcher started", "dirs", len(dirs), "watched", watched)
	return nil
}

// Stop shuts down the watcher.
func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.fsw.Close()
	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
}

func (w *Watcher) loop(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			slog.Warn("skills.watcher.error", "error", err)
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	path := event.Name

	// New directory → start watching it
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			_ = w.fsw.Add(path)
		}
	}

	// Only care about SKILL.md changes or directory removal
	base := filepath.Base(path)
	if !strings.EqualFold(base, "SKILL.md") && !event.Has(fsnotify.Create) {
		if !event.Has(fsnotify.Remove) && !event.Has(fsnotify.Rename) {
			return
		}
	}

	w.scheduleBump()
}

func (w *Watcher) scheduleBump() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(watchDebounce, func() {
		w.mu.Lock()
		if !w.pending {
			w.mu.Unlock()
			return
		}
		w.pending = false
		w.mu.Unlock()
		w.loader.BumpVersion()
		slog.Info("skills changed, version bumped", "version", w.loader.Version())
	})
}
