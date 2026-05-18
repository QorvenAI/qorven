// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package discussion

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var discSeq atomic.Uint64

const topicDriftThreshold = 0.65

// Embedder produces a vector for a text snippet.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// LabelGenerator generates a short label for a conversation excerpt.
type LabelGenerator interface {
	GenerateLabel(ctx context.Context, excerpt string) (string, error)
}

// StubEmbedder returns a fixed vector (for testing).
type StubEmbedder struct{ Vec []float32 }

func (s StubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.Vec, nil
}

// StubSequenceEmbedder returns vecs in order, cycling back to the last on exhaustion. For testing.
type StubSequenceEmbedder struct {
	Vecs  [][]float32
	index int
}

func (s *StubSequenceEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if len(s.Vecs) == 0 {
		return nil, nil
	}
	v := s.Vecs[s.index]
	if s.index < len(s.Vecs)-1 {
		s.index++
	}
	return v, nil
}

type Clusterer struct {
	mu       sync.Mutex
	store    *Store
	embedder Embedder
	labeller LabelGenerator
	vecCache map[string][]float32 // agentID → last embedding
	didCache map[string]string    // agentID → current discussion ID
}

func NewClusterer(pool *pgxpool.Pool, embedder Embedder, labeller LabelGenerator) *Clusterer {
	var store *Store
	if pool != nil {
		store = NewStore(pool)
	}
	return &Clusterer{
		store:    store,
		embedder: embedder,
		labeller: labeller,
		vecCache: make(map[string][]float32),
		didCache: make(map[string]string),
	}
}

// AssignDiscussion assigns a session to a discussion based on topic similarity.
// Returns (discussionID, wasCreated, error).
// Run in a goroutine — non-blocking by design.
//
// I/O (embed + label generation) is performed BEFORE acquiring the lock so the
// mutex is held only for the short cache read/write critical section.
func (c *Clusterer) AssignDiscussion(ctx context.Context, agentID, tenantID, sessionID, excerpt string) (string, bool, error) {
	// --- I/O outside the lock ---
	var vec []float32
	var embedErr error
	if c.embedder != nil {
		vec, embedErr = c.embedder.Embed(ctx, excerpt)
		if embedErr != nil {
			slog.Warn("clusterer.embed_failed", "agent", agentID, "error", embedErr)
		}
	}

	// Speculatively generate a label in case we need to create a new discussion.
	// We do this before acquiring the lock to avoid holding it during an LLM call.
	label := "New discussion · " + time.Now().Format("Jan 2")
	if c.labeller != nil {
		if l, err := c.labeller.GenerateLabel(ctx, excerpt); err == nil && l != "" {
			label = l
		}
	}

	// --- Lock only for cache reads/writes ---
	c.mu.Lock()
	defer c.mu.Unlock()

	// If embedding failed, fall back to the current discussion for this agent.
	if embedErr != nil {
		if did, ok := c.didCache[agentID]; ok {
			return did, false, nil
		}
	}

	if lastVec, ok := c.vecCache[agentID]; ok && vec != nil {
		sim := cosineSimilarity(vec, lastVec)
		if sim >= topicDriftThreshold {
			did := c.didCache[agentID]
			if c.store != nil {
				_ = c.store.Touch(ctx, tenantID, did)
				_ = c.store.AssignSession(ctx, sessionID, did)
			}
			c.vecCache[agentID] = vec
			return did, false, nil
		}
	}

	// Topic drifted or no existing discussion — create new one.
	var did string
	var err error
	if c.store != nil {
		did, err = c.store.Create(ctx, Discussion{
			TenantID: tenantID,
			AgentID:  agentID,
			AILabel:  label,
		})
		if err != nil {
			return "", false, err
		}
		_ = c.store.AssignSession(ctx, sessionID, did)
	} else {
		did = fmt.Sprintf("%s-disc-%d", agentID, discSeq.Add(1))
	}

	c.vecCache[agentID] = vec
	c.didCache[agentID] = did
	slog.Info("clusterer.new_discussion", "agent", agentID, "label", label, "id", did)
	return did, true, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
