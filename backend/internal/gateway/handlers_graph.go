// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handlers_graph.go — Knowledge Graph visualization API handlers.

func (gw *Gateway) handleGraphData(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 503, map[string]string{"error": "knowledge graph not configured"})
		return
	}

	data, err := gw.kgStore.GetGraphData(r.Context(), defaultTenant)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, data)
}

func (gw *Gateway) handleGraphNeighborhood(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 503, map[string]string{"error": "knowledge graph not configured"})
		return
	}

	nodeID := chi.URLParam(r, "nodeId")
	if nodeID == "" {
		writeJSON(w, 400, map[string]string{"error": "nodeId required"})
		return
	}

	depth := 1
	if r.URL.Query().Get("depth") == "2" {
		depth = 2
	}
	if r.URL.Query().Get("depth") == "3" {
		depth = 3
	}

	data, err := gw.kgStore.GetNodeNeighborhood(r.Context(), defaultTenant, nodeID, depth)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}
	writeJSON(w, 200, data)
}

func (gw *Gateway) handleGraphRelevance(w http.ResponseWriter, r *http.Request) {
	if gw.kgStore == nil {
		writeJSON(w, 503, map[string]string{"error": "knowledge graph not configured"})
		return
	}

	nodeID := chi.URLParam(r, "nodeId")
	if nodeID == "" {
		writeJSON(w, 400, map[string]string{"error": "nodeId required"})
		return
	}

	edges, err := gw.kgStore.GetRelevanceEdges(r.Context(), defaultTenant, 50)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": sanitizeError(err)})
		return
	}

	// Filter to edges involving this node
	relevant := []any{}
	for _, e := range edges {
		if e.Source == nodeID || e.Target == nodeID {
			relevant = append(relevant, e)
		}
	}
	writeJSON(w, 200, map[string]any{"node_id": nodeID, "edges": relevant})
}
