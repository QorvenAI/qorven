// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// pipeline.go — Parallel search orchestration, result fusion, and ranking.

type Source interface {
	Name() string
	Search(ctx context.Context, topic string, depth string) ([]SourceItem, error)
}

type Pipeline struct {
	sources    []Source
	maxWorkers int
}

func NewPipeline(sources ...Source) *Pipeline {
	return &Pipeline{sources: sources, maxWorkers: 10}
}

func (p *Pipeline) AddSource(s Source) { p.sources = append(p.sources, s) }

// Run executes the full pipeline: plan → retrieve → fuse → rank → report.
func (p *Pipeline) Run(ctx context.Context, topic string, depth string) (*Report, error) {
	start := time.Now()

	// Parallel retrieval from all sources
	bundle := p.retrieve(ctx, topic, depth)

	// Normalize and deduplicate
	candidates := p.fuse(bundle, topic)

	// Rank by engagement + relevance
	p.rank(candidates, topic)

	// Cluster related results
	clusters := p.cluster(candidates)

	// Build report
	now := time.Now()
	report := &Report{
		Topic:       topic,
		RangeFrom:   now.Add(-30 * 24 * time.Hour).Format("2006-01-02"),
		RangeTo:     now.Format("2006-01-02"),
		GeneratedAt: now.Format(time.RFC3339),
		Clusters:        clusters,
		RankedCandidates: candidates,
		ItemsBySource:    bundle.ItemsBySource,
		ErrorsBySource:   bundle.ErrorsBySource,
	}

	log.Printf("pipeline: %d sources, %d items, %d candidates, %d clusters in %v",
		len(p.sources), countItems(bundle), len(candidates), len(clusters), time.Since(start).Round(time.Millisecond))

	return report, nil
}

func (p *Pipeline) retrieve(ctx context.Context, topic string, depth string) *RetrievalBundle {
	bundle := NewRetrievalBundle()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.maxWorkers)

	for _, src := range p.sources {
		wg.Add(1)
		go func(s Source) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			items, err := s.Search(ctx, topic, depth)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				bundle.ErrorsBySource[s.Name()] = err.Error()
				return
			}
			bundle.AddItems(topic, s.Name(), items)
		}(src)
	}
	wg.Wait()
	return bundle
}

func (p *Pipeline) fuse(bundle *RetrievalBundle, topic string) []Candidate {
	seen := map[string]bool{}
	var candidates []Candidate
	idx := 0

	for source, items := range bundle.ItemsBySource {
		for rank, item := range items {
			// Dedup by URL
			key := item.URL
			if key == "" { key = item.ItemID }
			if seen[key] { continue }
			seen[key] = true

			

			engagement := 0.0
			for _, v := range item.Engagement { engagement += v }

			candidates = append(candidates, Candidate{
				CandidateID:    fmt.Sprintf("c_%d", idx),
				ItemID:         item.ItemID,
				Source:         source,
				Title:          item.Title,
				URL:            item.URL,
				Snippet:        truncateStr(item.Body, 300),
				SubQueryLabels: []string{topic},
				NativeRanks:    map[string]int{source: rank},
				LocalRelevance: item.RelevanceHint,
				Freshness:      computeFreshness(item.PublishedAt),
				Engagement:     engagement,
				SourceQuality:  sourceQuality(source),
				RRFScore:       rrfScore(rank),
				Sources:        []string{source},
				SourceItems:    []SourceItem{item},
			})
			idx++
		}
	}
	return candidates
}

func (p *Pipeline) rank(candidates []Candidate, topic string) {
	for i := range candidates {
		c := &candidates[i]
		// Final score: weighted combination
		c.FinalScore = c.RRFScore*0.15 +
			c.LocalRelevance*0.25 +
			normalizeEng(c.Engagement)*0.35 +
			float64(c.Freshness)*0.001*0.15 +
			c.SourceQuality*0.10
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].FinalScore > candidates[j].FinalScore
	})
}

func (p *Pipeline) cluster(candidates []Candidate) []Cluster {
	// Simple clustering: group by source, then by title similarity
	bySource := map[string][]string{}
	for _, c := range candidates {
		bySource[c.Source] = append(bySource[c.Source], c.CandidateID)
	}

	var clusters []Cluster
	idx := 0
	for source, ids := range bySource {
		if len(ids) == 0 { continue }
		rep := ids
		if len(rep) > 3 { rep = rep[:3] }
		clusters = append(clusters, Cluster{
			ClusterID:         fmt.Sprintf("cl_%d", idx),
			Title:             strings.Title(source) + " Results",
			CandidateIDs:      ids,
			RepresentativeIDs: rep,
			Sources:           []string{source},
			Score:             float64(len(ids)),
		})
		idx++
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].Score > clusters[j].Score })
	return clusters
}

// ── Scoring helpers ──

func rrfScore(rank int) float64 {
	k := 60.0 // RRF constant
	return 1.0 / (k + float64(rank))
}



func countItems(b *RetrievalBundle) int {
	n := 0
	for _, items := range b.ItemsBySource { n += len(items) }
	return n
}
