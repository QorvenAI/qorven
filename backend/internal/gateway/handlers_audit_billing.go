// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/audit"
)

func (gw *Gateway) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only log mutating requests
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		if gw.auditStore != nil {
			// Extract resource from path: /v1/agents/123 → resource=agents, id=123
			path := r.URL.Path
			resource, resourceID := parseAuditPath(path)
			action := methodToAction(r.Method)

			gw.auditStore.Log(r.Context(), defaultTenant,
				audit.ActorUser, "api", "",
				action, resource, resourceID,
				map[string]string{"method": r.Method, "path": path, "query": r.URL.RawQuery},
				r.RemoteAddr,
			)
		}
		next.ServeHTTP(w, r)
	})
}

func parseAuditPath(path string) (resource, resourceID string) {
	// /v1/agents/abc123 → agents, abc123
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		resource = parts[1] // skip "v1"
		if len(parts) >= 3 {
			resourceID = parts[2]
		}
	}
	if resource == "" {
		resource = path
	}
	return
}

func methodToAction(method string) string {
	switch method {
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return method
	}
}

func (gw *Gateway) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if gw.auditStore == nil {
		writeJSON(w, 503, map[string]string{"error": "audit not configured"})
		return
	}
	q := r.URL.Query()
	opts := audit.QueryOpts{
		ActorID:  q.Get("actor_id"),
		Resource: q.Get("resource"),
		Action:   q.Get("action"),
		Limit:    50,
		Offset:   0,
	}
	if l := q.Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &opts.Limit)
	}
	if o := q.Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &opts.Offset)
	}
	entries, total, err := gw.auditStore.Query(r.Context(), defaultTenant, opts)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries, "total": total})
}

func (gw *Gateway) handleBillingCosts(w http.ResponseWriter, r *http.Request) {
	if gw.billingStore == nil {
		writeJSON(w, 503, map[string]string{"error": "billing not configured"})
		return
	}
	since := time.Now().AddDate(0, -1, 0) // last 30 days
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		}
	}
	costs, _ := gw.billingStore.GetAgentCosts(r.Context(), defaultTenant, since)
	total, count, _ := gw.billingStore.GetTotalCost(r.Context(), defaultTenant, since)
	recent, _ := gw.billingStore.RecentEvents(r.Context(), defaultTenant, 50)
	writeJSON(w, 200, map[string]any{
		"agent_costs": costs,
		"total_cents": total,
		"total_calls": count,
		"recent":      recent,
		"since":       since,
	})
}
