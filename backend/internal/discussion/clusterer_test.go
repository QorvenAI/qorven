package discussion_test

import (
	"context"
	"testing"

	"github.com/qorvenai/qorven/internal/discussion"
)

func TestClusterer_SameTopicExtendsDiscussion(t *testing.T) {
	// StubEmbedder returns same vector → cosine sim = 1.0 → same discussion
	embedder := discussion.StubEmbedder{Vec: []float32{1, 0, 0}}
	c := discussion.NewClusterer(nil, embedder, nil)
	ctx := context.Background()

	did1, created1, err := c.AssignDiscussion(ctx, "agent-1", "tenant-1", "session-1",
		"Tell me about Tamil Nadu elections")
	if err != nil {
		t.Fatalf("AssignDiscussion: %v", err)
	}
	if !created1 {
		t.Error("expected first call to create a new discussion")
	}

	did2, created2, err := c.AssignDiscussion(ctx, "agent-1", "tenant-1", "session-1",
		"What about NTK election results")
	if err != nil {
		t.Fatalf("AssignDiscussion second: %v", err)
	}
	if created2 {
		t.Error("expected second call to reuse existing discussion")
	}
	if did1 != did2 {
		t.Errorf("discussion IDs differ: %q vs %q", did1, did2)
	}
}

func TestClusterer_TopicDriftCreatesNewDiscussion(t *testing.T) {
	// First call with vec {1,0,0}, second with {0,1,0} → cosine sim = 0 → new discussion
	c := discussion.NewClusterer(nil, nil, nil) // will panic on embedder — use manual vec injection via StubEmbedder
	_ = c
	// Actually use two different StubEmbedders via the Clusterer's in-memory cache behavior:
	// We can't change the embedder mid-flight, so test via orthogonal vectors:
	// Trick: use a custom embedder that returns different vectors per call count
	counter := 0
	_ = counter
	vecs := [][]float32{{1, 0, 0}, {0, 1, 0}}
	_ = vecs
	// Use a closure-based approach via StubSequence
	seq := &discussion.StubSequenceEmbedder{Vecs: vecs}
	c2 := discussion.NewClusterer(nil, seq, nil)

	ctx := context.Background()
	did1, created1, _ := c2.AssignDiscussion(ctx, "agent-2", "tenant-1", "s1", "election results")
	if !created1 {
		t.Error("expected first discussion created")
	}
	did2, created2, _ := c2.AssignDiscussion(ctx, "agent-2", "tenant-1", "s2", "recipe for dosa")
	if !created2 {
		t.Error("expected new discussion on topic drift")
	}
	if did1 == did2 {
		t.Error("expected different discussion IDs for drifted topics")
	}
}

func TestCosineSimilarity_Orthogonal(t *testing.T) {
	// Public wrapper not exported — test via StubEmbedder behavior in Clusterer
	// Test that orthogonal vectors cause a new discussion (indirect test of cosineSimilarity)
	seq := &discussion.StubSequenceEmbedder{Vecs: [][]float32{{1, 0}, {0, 1}}}
	c := discussion.NewClusterer(nil, seq, nil)
	ctx := context.Background()

	_, created1, _ := c.AssignDiscussion(ctx, "agent-3", "t1", "s1", "topic A")
	_, created2, _ := c.AssignDiscussion(ctx, "agent-3", "t1", "s2", "topic B")

	if !created1 || !created2 {
		t.Errorf("both should create new discussions, got created1=%v created2=%v", created1, created2)
	}
}
