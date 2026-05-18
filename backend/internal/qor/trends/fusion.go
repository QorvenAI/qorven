// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package trends

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// fusion.go — Reciprocal Rank Fusion, URL normalization, cross-source dedup.
// Rewritten from last30days fusion.py (202 lines).

const rrfK = 60.0

// FuseResults merges items from multiple sources into a unified candidate list.
func FuseResults(bundle *RetrievalBundle, plan QueryPlan) []Candidate {
	seen := map[string]bool{}
	var candidates []Candidate
	idx := 0

	for _, sq := range plan.SubQueries {
		for _, source := range sq.Sources {
			key := sq.Label + "|" + source
			items := bundle.ItemsBySourceAndQuery[key]
			sourceWeight := plan.SourceWeights[source]
			if sourceWeight == 0 { sourceWeight = 1.0 }

			for rank, item := range items {
				normURL := NormalizeURL(item.URL)
				if normURL == "" { normURL = item.ItemID }
				if seen[normURL] { continue }
				seen[normURL] = true

				engagement := 0.0
				for _, v := range item.Engagement { engagement += v }

				candidates = append(candidates, Candidate{
					CandidateID:    fmt.Sprintf("c_%d", idx),
					ItemID:         item.ItemID,
					Source:         source,
					Title:          item.Title,
					URL:            item.URL,
					Snippet:        truncateStr(item.Body, 300),
					SubQueryLabels: []string{sq.Label},
					NativeRanks:    map[string]int{source: rank},
					LocalRelevance: item.RelevanceHint,
					Freshness:      computeFreshness(item.PublishedAt),
					Engagement:     engagement,
					SourceQuality:  sourceQuality(source),
					RRFScore:       (1.0 / (rrfK + float64(rank))) * sourceWeight * sq.Weight,
					Sources:        []string{source},
					SourceItems:    []SourceItem{item},
				})
				idx++
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidateSortKey(candidates[i]) > candidateSortKey(candidates[j])
	})
	return candidates
}

func candidateSortKey(c Candidate) float64 {
	return c.RRFScore*1000 + c.LocalRelevance*100 + float64(c.Freshness)
}

// NormalizeURL canonicalizes a URL for dedup: lowercase, strip www/old/m prefixes, remove tracking params.
func NormalizeURL(rawURL string) string {
	if rawURL == "" { return "" }
	parsed, err := url.Parse(strings.ToLower(strings.TrimSpace(rawURL)))
	if err != nil { return rawURL }

	// Strip www/old/m prefixes
	host := parsed.Hostname()
	for _, prefix := range []string{"www.", "old.", "m.", "mobile."} {
		host = strings.TrimPrefix(host, prefix)
	}

	// Remove tracking params
	q := parsed.Query()
	for _, param := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term", "ref", "fbclid", "gclid", "mc_cid", "mc_eid"} {
		q.Del(param)
	}

	return host + parsed.Path + func() string {
		encoded := q.Encode()
		if encoded == "" { return "" }
		return "?" + encoded
	}()
}
