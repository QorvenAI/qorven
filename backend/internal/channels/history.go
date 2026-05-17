// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	maxHistoryKeys           = 1000
	DefaultGroupHistoryLimit = 200
	flushInterval            = 3 * time.Second
	flushBatchMax            = 20
	compactSweepInterval     = 10 * time.Minute
)

// PendingMessageStore interface for persistent history storage.
type PendingMessageStore interface {
	AppendBatch(ctx context.Context, msgs []PendingMessage) error
	ListByKey(ctx context.Context, channelName, historyKey string) ([]PendingMessage, error)
	DeleteByKey(ctx context.Context, channelName, historyKey string) error
	CountByKey(ctx context.Context, channelName, historyKey string) (int, error)
	Compact(ctx context.Context, deleteIDs []uuid.UUID, summary *PendingMessage) error
	ListGroups(ctx context.Context) ([]PendingGroupInfo, error)
}

// PendingMessage represents a message in persistent storage.
type PendingMessage struct {
	ID            uuid.UUID
	ChannelName   string
	HistoryKey    string
	Sender        string
	SenderID      string
	Body          string
	PlatformMsgID string
	IsSummary     bool
	CreatedAt     time.Time
}

// PendingGroupInfo represents a group's message count.
type PendingGroupInfo struct {
	ChannelName  string
	HistoryKey   string
	MessageCount int
}

// MediaRef is a lightweight reference to platform media for deferred download.
type MediaRef struct {
	Type     string
	FileID   string
	FileSize int64
}

// HistoryEntry represents a single tracked group message.
type HistoryEntry struct {
	Sender    string
	SenderID  string
	Body      string
	Media     []string
	MediaRefs []MediaRef
	Timestamp time.Time
	MessageID string
}

// PendingHistory tracks group messages across multiple groups.
type PendingHistory struct {
	mu          sync.Mutex
	entries     map[string][]HistoryEntry
	order       []string
	tenantID    uuid.UUID
	channelName string
	store       PendingMessageStore
	flushMu     sync.Mutex
	flushBuf    []PendingMessage
	flushSignal chan struct{}
	stopCh      chan struct{}
	stopped     chan struct{}
	compactCfg  *CompactionConfig
	compacting  sync.Map
}

// NewPendingHistory creates a new RAM-only pending history tracker.
func NewPendingHistory() *PendingHistory {
	return &PendingHistory{entries: make(map[string][]HistoryEntry)}
}

// NewPersistentHistory creates a persistent history tracker with batched DB flush.
func NewPersistentHistory(channelName string, store PendingMessageStore, tenantID uuid.UUID) *PendingHistory {
	return &PendingHistory{
		entries:     make(map[string][]HistoryEntry),
		channelName: channelName,
		store:       store,
		tenantID:    tenantID,
		flushSignal: make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		stopped:     make(chan struct{}),
	}
}

// IsPersistent returns true if this history is backed by a DB store.
func (ph *PendingHistory) IsPersistent() bool { return ph.store != nil }

// SetTenantID updates the tenant scope.
func (ph *PendingHistory) SetTenantID(id uuid.UUID) { ph.tenantID = id }

// SetCompactionConfig sets the LLM compaction config.
func (ph *PendingHistory) SetCompactionConfig(cfg *CompactionConfig) { ph.compactCfg = cfg }

// StartFlusher starts the background DB flush goroutine.
func (ph *PendingHistory) StartFlusher() {
	if ph.store == nil {
		return
	}
	go ph.flushLoop()
}

// StopFlusher stops the background flusher.
func (ph *PendingHistory) StopFlusher() {
	if ph.store == nil {
		return
	}
	close(ph.stopCh)
	<-ph.stopped
}

// Record adds a message to the pending history for a group.
func (ph *PendingHistory) Record(historyKey string, entry HistoryEntry, limit int) {
	if limit <= 0 || historyKey == "" {
		return
	}

	var count int

	ph.mu.Lock()
	existing := ph.entries[historyKey]
	existing = append(existing, entry)
	count = len(existing)
	if len(existing) > limit {
		trimmed := existing[:len(existing)-limit]
		go cleanupMedia(trimmed)
		existing = existing[len(existing)-limit:]
	}
	ph.entries[historyKey] = existing
	ph.removeFromOrder(historyKey)
	ph.order = append(ph.order, historyKey)
	ph.evictOldKeys()
	ph.mu.Unlock()

	// Queue for DB persistence
	if ph.store != nil {
		ph.enqueueFlush(PendingMessage{
			ChannelName:   ph.channelName,
			HistoryKey:    historyKey,
			Sender:        entry.Sender,
			SenderID:      entry.SenderID,
			Body:          entry.Body,
			PlatformMsgID: entry.MessageID,
			CreatedAt:     entry.Timestamp,
		})
	}

	// Trigger compaction if threshold exceeded
	ph.MaybeCompact(historyKey, count)
}

// BuildContext retrieves pending history and formats it as context.
func (ph *PendingHistory) BuildContext(historyKey, currentMessage string, limit int) string {
	if limit <= 0 || historyKey == "" {
		return currentMessage
	}

	ph.mu.Lock()
	entries := ph.entries[historyKey]
	entriesCopy := make([]HistoryEntry, len(entries))
	copy(entriesCopy, entries)
	ph.mu.Unlock()

	// DB fallback if RAM is empty
	if len(entriesCopy) == 0 && ph.store != nil {
		entriesCopy = ph.loadFromDB(historyKey)
	}

	if len(entriesCopy) == 0 {
		return currentMessage
	}

	lines := []string{}
	for _, e := range entriesCopy {
		ts := ""
		if !e.Timestamp.IsZero() {
			ts = fmt.Sprintf(" [%s]", e.Timestamp.Format("15:04"))
		}
		lines = append(lines, fmt.Sprintf("  %s%s: %s", e.Sender, ts, e.Body))
	}

	return fmt.Sprintf("[Chat messages since your last reply - for context]\n%s\n\n[Your current message]\n%s",
		strings.Join(lines, "\n"), currentMessage)
}

func (ph *PendingHistory) loadFromDB(historyKey string) []HistoryEntry {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := ph.store.ListByKey(ctx, ph.channelName, historyKey)
	if err != nil || len(msgs) == 0 {
		return nil
	}

	entries := make([]HistoryEntry, 0, len(msgs))
	for _, m := range msgs {
		entries = append(entries, HistoryEntry{
			Sender:    m.Sender,
			SenderID:  m.SenderID,
			Body:      m.Body,
			Timestamp: m.CreatedAt,
			MessageID: m.PlatformMsgID,
		})
	}
	return entries
}

// GetEntries returns a copy of pending entries for a group.
func (ph *PendingHistory) GetEntries(historyKey string) []HistoryEntry {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	entries := ph.entries[historyKey]
	if len(entries) == 0 {
		return nil
	}
	result := make([]HistoryEntry, len(entries))
	copy(result, entries)
	return result
}

// Clear removes all pending history for a group.
func (ph *PendingHistory) Clear(historyKey string) {
	if historyKey == "" {
		return
	}

	ph.mu.Lock()
	toClean := ph.entries[historyKey]
	delete(ph.entries, historyKey)
	ph.removeFromOrder(historyKey)
	ph.mu.Unlock()

	go cleanupMedia(toClean)

	if ph.store != nil {
		ph.removeFromFlushBuf(historyKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ph.store.DeleteByKey(ctx, ph.channelName, historyKey)
	}
}

// CollectMedia returns all media file paths from pending entries.
func (ph *PendingHistory) CollectMedia(historyKey string) []string {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	entries := ph.entries[historyKey]
	paths := []string{}
	for i := range entries {
		paths = append(paths, entries[i].Media...)
		entries[i].Media = nil
	}
	return paths
}

// CollectMediaRefs returns all deferred media references from pending entries.
func (ph *PendingHistory) CollectMediaRefs(historyKey string) []MediaRef {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	entries := ph.entries[historyKey]
	refs := []MediaRef{}
	for i := range entries {
		refs = append(refs, entries[i].MediaRefs...)
		entries[i].MediaRefs = nil
	}
	return refs
}

func (ph *PendingHistory) removeFromOrder(key string) {
	for i, k := range ph.order {
		if k == key {
			ph.order = append(ph.order[:i], ph.order[i+1:]...)
			return
		}
	}
}

func (ph *PendingHistory) evictOldKeys() {
	for len(ph.order) > maxHistoryKeys {
		oldest := ph.order[0]
		ph.order = ph.order[1:]
		evicted := ph.entries[oldest]
		delete(ph.entries, oldest)
		go cleanupMedia(evicted)
	}
}

func cleanupMedia(entries []HistoryEntry) {
	for _, e := range entries {
		for _, path := range e.Media {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				slog.Warn("pending_history: media cleanup failed", "path", path, "error", err)
			}
		}
	}
}

// --- Flush ---

func (ph *PendingHistory) enqueueFlush(msg PendingMessage) {
	ph.flushMu.Lock()
	ph.flushBuf = append(ph.flushBuf, msg)
	shouldSignal := len(ph.flushBuf) >= flushBatchMax
	ph.flushMu.Unlock()

	if shouldSignal {
		select {
		case ph.flushSignal <- struct{}{}:
		default:
		}
	}
}

func (ph *PendingHistory) removeFromFlushBuf(historyKey string) {
	ph.flushMu.Lock()
	defer ph.flushMu.Unlock()

	filtered := ph.flushBuf[:0]
	for _, msg := range ph.flushBuf {
		if msg.HistoryKey != historyKey {
			filtered = append(filtered, msg)
		}
	}
	ph.flushBuf = filtered
}

func (ph *PendingHistory) flushNow() {
	ph.flushMu.Lock()
	batch := ph.flushBuf
	ph.flushBuf = nil
	ph.flushMu.Unlock()

	if len(batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := ph.store.AppendBatch(ctx, batch); err != nil {
		slog.Warn("pending_history.flush_failed", "channel", ph.channelName, "batch_size", len(batch), "error", err)
	}
}

func (ph *PendingHistory) flushLoop() {
	defer close(ph.stopped)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	compactTicker := time.NewTicker(compactSweepInterval)
	defer compactTicker.Stop()

	for {
		select {
		case <-ph.stopCh:
			ph.flushNow()
			return
		case <-ph.flushSignal:
			ph.flushNow()
		case <-ticker.C:
			ph.flushNow()
		case <-compactTicker.C:
			ph.sweepCompaction()
		}
	}
}

// --- Compaction ---

// MaybeCompact checks if compaction is needed and triggers it in background.
func (ph *PendingHistory) MaybeCompact(historyKey string, currentCount int) {
	if ph.store == nil || ph.compactCfg == nil || ph.compactCfg.Summarize == nil {
		return
	}
	threshold := ph.compactCfg.Threshold
	if threshold <= 0 {
		threshold = DefaultGroupHistoryLimit
	}
	if currentCount <= threshold {
		return
	}

	if _, loaded := ph.compacting.LoadOrStore(historyKey, true); loaded {
		return
	}
	go ph.runCompaction(historyKey)
}

func (ph *PendingHistory) sweepCompaction() {
	if ph.compactCfg == nil || ph.compactCfg.Summarize == nil || ph.store == nil {
		return
	}
	threshold := ph.compactCfg.Threshold
	if threshold <= 0 {
		threshold = DefaultGroupHistoryLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	groups, err := ph.store.ListGroups(ctx)
	if err != nil {
		return
	}
	for _, g := range groups {
		if g.ChannelName != ph.channelName {
			continue
		}
		if g.MessageCount > threshold {
			ph.MaybeCompact(g.HistoryKey, g.MessageCount)
		}
	}
}

func (ph *PendingHistory) runCompaction(historyKey string) {
	defer ph.compacting.Delete(historyKey)

	ph.flushNow()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	entries, err := ph.store.ListByKey(ctx, ph.channelName, historyKey)
	if err != nil {
		return
	}

	threshold := ph.compactCfg.Threshold
	if threshold <= 0 {
		threshold = DefaultGroupHistoryLimit
	}
	if len(entries) <= threshold {
		return
	}

	keepRecent := 40
	if keepRecent >= len(entries) {
		return
	}

	splitIdx := len(entries) - keepRecent
	toSummarize := entries[:splitIdx]

	var sb strings.Builder
	for _, e := range toSummarize {
		prefix := e.Sender
		if e.IsSummary {
			prefix = "[previous summary]"
		}
		fmt.Fprintf(&sb, "%s: %s\n", prefix, e.Body)
	}

	summary, err := ph.compactCfg.Summarize(ctx, sb.String())
	if err != nil {
		slog.Warn("compaction.failed", "channel", ph.channelName, "key", historyKey, "error", err)
		return
	}

	deleteIDs := make([]uuid.UUID, len(toSummarize))
	for i, e := range toSummarize {
		deleteIDs[i] = e.ID
	}

	summaryMsg := &PendingMessage{
		ChannelName: ph.channelName,
		HistoryKey:  historyKey,
		Sender:      "[summary]",
		Body:        summary,
		IsSummary:   true,
	}

	if err := ph.store.Compact(ctx, deleteIDs, summaryMsg); err != nil {
		slog.Warn("compaction.db_failed", "channel", ph.channelName, "key", historyKey, "error", err)
		return
	}

	slog.Info("compaction.done", "channel", ph.channelName, "key", historyKey, "summarized", len(toSummarize), "kept", keepRecent)
}

// --- Types ---

// ThinkTagSplit holds the result of parsing <think> tags from content.
type ThinkTagSplit struct {
	Thinking string
	Answer   string
	Partial  bool
}

// SplitThinkTags parses <think>...</think> tags from content.
func SplitThinkTags(content string) ThinkTagSplit {
	const openTag = "<think>"
	const closeTag = "</think>"

	openIdx := strings.Index(content, openTag)
	if openIdx == -1 {
		return ThinkTagSplit{Answer: content}
	}

	closeIdx := strings.Index(content, closeTag)
	if closeIdx == -1 {
		return ThinkTagSplit{
			Thinking: content[openIdx+len(openTag):],
			Partial:  true,
		}
	}

	thinking := content[openIdx+len(openTag) : closeIdx]
	answer := strings.TrimSpace(content[closeIdx+len(closeTag):])
	return ThinkTagSplit{
		Thinking: thinking,
		Answer:   answer,
	}
}

// PendingCompactable is optionally implemented by channels with PendingHistory.
type PendingCompactable interface {
	SetPendingCompaction(cfg *CompactionConfig)
}

// CompactionConfig holds LLM compaction settings.
type CompactionConfig struct {
	Threshold int
	KeepRecent int
	MaxTokens int
	Summarize func(ctx context.Context, text string) (string, error)
}
