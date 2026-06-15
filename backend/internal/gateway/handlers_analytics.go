// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// handleAnalyticsOverview returns aggregate content performance stats for the
// marketing analytics dashboard. It queries the outbound_queue and social_posts
// tables for production, approval, and publication counts.
func (gw *Gateway) handleAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	ctx := r.Context()
	pool := gw.db.Pool

	// Content produced (from outbound_queue)
	var produced7d, produced30d int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE requested_at > NOW() - INTERVAL '7 days'`).Scan(&produced7d)
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE requested_at > NOW() - INTERVAL '30 days'`).Scan(&produced30d)

	// Approval stats
	var approved7d, rejected7d int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE status = 'approved' AND reviewed_at > NOW() - INTERVAL '7 days'`).Scan(&approved7d)
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM outbound_queue WHERE status = 'rejected' AND reviewed_at > NOW() - INTERVAL '7 days'`).Scan(&rejected7d)

	// Published posts
	var published7d int
	pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM social_posts WHERE status = 'published' AND published_at > NOW() - INTERVAL '7 days'`).Scan(&published7d)

	// Posts by platform (use first element of platforms jsonb array).
	// Returned as a {platform: count} map — the analytics UI iterates it with
	// Object.entries(), so the shape must be an object, not an array.
	postsByPlatform := map[string]int{}
	rows, err := pool.Query(ctx,
		`SELECT platforms->>0 AS platform, COUNT(*) AS cnt
		 FROM social_posts
		 WHERE status = 'published' AND published_at > NOW() - INTERVAL '30 days' AND platforms IS NOT NULL
		 GROUP BY platforms->>0
		 ORDER BY cnt DESC`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var platform string
			var cnt int
			rows.Scan(&platform, &cnt)
			if platform != "" {
				postsByPlatform[platform] = cnt
			}
		}
	}

	// Posts by agent (join with agents for the display name). The JSON field is
	// "agent_name" to match what the analytics UI reads.
	type agentCount struct {
		AgentID     string `json:"agent_id"`
		DisplayName string `json:"agent_name"`
		Count       int    `json:"count"`
	}
	postsByAgent := []agentCount{}
	rows2, err := pool.Query(ctx,
		`SELECT sp.agent_id, COALESCE(a.display_name, a.agent_key) AS name, COUNT(*) AS cnt
		 FROM social_posts sp
		 LEFT JOIN agents a ON a.id = sp.agent_id
		 WHERE sp.status = 'published' AND sp.published_at > NOW() - INTERVAL '30 days'
		 GROUP BY sp.agent_id, name
		 ORDER BY cnt DESC`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var ac agentCount
			rows2.Scan(&ac.AgentID, &ac.DisplayName, &ac.Count)
			postsByAgent = append(postsByAgent, ac)
		}
	}

	// Approval rate
	var approvalRate float64
	total := approved7d + rejected7d
	if total > 0 {
		approvalRate = float64(approved7d) / float64(total) * 100
	}

	writeJSON(w, 200, map[string]any{
		"content_produced_7d":  produced7d,
		"content_produced_30d": produced30d,
		"approved_7d":          approved7d,
		"rejected_7d":          rejected7d,
		"published_7d":         published7d,
		"posts_by_platform":    postsByPlatform,
		"posts_by_agent":       postsByAgent,
		"approval_rate":        approvalRate,
	})
}

// handleAnalyticsSEO returns Google Search Console data if the connector is
// configured, or a graceful "not connected" response otherwise.
func (gw *Gateway) handleAnalyticsSEO(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	ctx := r.Context()

	// Check if Google Search Console token exists in vault
	if gw.vault == nil {
		writeJSON(w, 200, map[string]any{"connected": false, "connect_url": "/settings/integrations"})
		return
	}
	_, err := gw.vault.GetToken(ctx, defaultTenant, "google-search-console", nil)
	if err != nil {
		writeJSON(w, 200, map[string]any{"connected": false, "connect_url": "/settings/integrations"})
		return
	}

	// Connected — attempt to fetch data via connector executor
	if gw.connExec == nil {
		writeJSON(w, 200, map[string]any{
			"connected":   true,
			"clicks":      0,
			"impressions": 0,
			"ctr":         0,
			"position":    0,
			"top_queries": []any{},
			"note":        "connector executor not available",
		})
		return
	}

	result, err := gw.connExec.Execute(ctx, "google-search-console", "search_analytics", map[string]any{
		"start_date": time.Now().AddDate(0, 0, -28).Format("2006-01-02"),
		"end_date":   time.Now().Format("2006-01-02"),
		"dimensions": []string{"query"},
		"row_limit":  10,
	})
	if err != nil {
		// Connected but fetch failed — return partial
		writeJSON(w, 200, map[string]any{
			"connected":   true,
			"clicks":      0,
			"impressions": 0,
			"ctr":         0,
			"position":    0,
			"top_queries": []any{},
			"error":       sanitizeError(err),
		})
		return
	}

	writeJSON(w, 200, map[string]any{
		"connected": true,
		"raw_data":  result,
	})
}

// handleAnalyticsTraffic returns Google Analytics data if connected, or a
// graceful placeholder otherwise.
func (gw *Gateway) handleAnalyticsTraffic(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	ctx := r.Context()

	// Check if Google Analytics token exists in vault
	if gw.vault == nil {
		writeJSON(w, 200, map[string]any{"connected": false, "connect_url": "/settings/integrations"})
		return
	}
	_, err := gw.vault.GetToken(ctx, defaultTenant, "google-analytics", nil)
	if err != nil {
		writeJSON(w, 200, map[string]any{"connected": false, "connect_url": "/settings/integrations"})
		return
	}

	// Connected — attempt to fetch data via connector executor
	if gw.connExec == nil {
		writeJSON(w, 200, map[string]any{
			"connected":    true,
			"sessions_7d":  0,
			"users_7d":     0,
			"pageviews_7d": 0,
			"top_pages":    []any{},
			"note":         "connector executor not available",
		})
		return
	}

	result, err := gw.connExec.Execute(ctx, "google-analytics", "run_report", map[string]any{
		"start_date": time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
		"end_date":   time.Now().Format("2006-01-02"),
		"metrics":    []string{"sessions", "totalUsers", "screenPageViews"},
		"dimensions": []string{"pagePath"},
		"limit":      10,
	})
	if err != nil {
		writeJSON(w, 200, map[string]any{
			"connected":    true,
			"sessions_7d":  0,
			"users_7d":     0,
			"pageviews_7d": 0,
			"top_pages":    []any{},
			"error":        sanitizeError(err),
		})
		return
	}

	writeJSON(w, 200, map[string]any{
		"connected": true,
		"raw_data":  result,
	})
}

// handleAnalyticsTimeline returns daily aggregation data for charting content
// production and publishing over time.
func (gw *Gateway) handleAnalyticsTimeline(w http.ResponseWriter, r *http.Request) {
	if gw.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database not available"})
		return
	}
	ctx := r.Context()
	pool := gw.db.Pool

	// Parse days parameter (default 30)
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 365 {
			days = parsed
		}
	}

	type dayRow struct {
		Date      string `json:"date"`
		Produced  int    `json:"produced"`
		Approved  int    `json:"approved"`
		Published int    `json:"published"`
		Rejected  int    `json:"rejected"`
	}

	query := fmt.Sprintf(`
		WITH dates AS (
			SELECT generate_series(
				(NOW() - INTERVAL '%d days')::date,
				NOW()::date,
				'1 day'::interval
			)::date AS d
		),
		produced AS (
			SELECT requested_at::date AS d, COUNT(*) AS cnt
			FROM outbound_queue
			WHERE requested_at > NOW() - INTERVAL '%d days'
			GROUP BY requested_at::date
		),
		approved AS (
			SELECT reviewed_at::date AS d, COUNT(*) AS cnt
			FROM outbound_queue
			WHERE status = 'approved' AND reviewed_at > NOW() - INTERVAL '%d days'
			GROUP BY reviewed_at::date
		),
		rejected AS (
			SELECT reviewed_at::date AS d, COUNT(*) AS cnt
			FROM outbound_queue
			WHERE status = 'rejected' AND reviewed_at > NOW() - INTERVAL '%d days'
			GROUP BY reviewed_at::date
		),
		published AS (
			SELECT published_at::date AS d, COUNT(*) AS cnt
			FROM social_posts
			WHERE status = 'published' AND published_at > NOW() - INTERVAL '%d days'
			GROUP BY published_at::date
		)
		SELECT
			dates.d::text,
			COALESCE(produced.cnt, 0),
			COALESCE(approved.cnt, 0),
			COALESCE(published.cnt, 0),
			COALESCE(rejected.cnt, 0)
		FROM dates
		LEFT JOIN produced ON produced.d = dates.d
		LEFT JOIN approved ON approved.d = dates.d
		LEFT JOIN published ON published.d = dates.d
		LEFT JOIN rejected ON rejected.d = dates.d
		ORDER BY dates.d
	`, days, days, days, days, days)

	rows, err := pool.Query(ctx, query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeError(err)})
		return
	}
	defer rows.Close()

	timeline := []dayRow{}
	for rows.Next() {
		var dr dayRow
		if err := rows.Scan(&dr.Date, &dr.Produced, &dr.Approved, &dr.Published, &dr.Rejected); err != nil {
			continue
		}
		timeline = append(timeline, dr)
	}

	// Return the raw array — the analytics UI consumes /analytics/timeline as a
	// flat TimelineDay[], not a wrapped object.
	writeJSON(w, 200, timeline)
}
