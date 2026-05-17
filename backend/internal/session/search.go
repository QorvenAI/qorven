// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SearchResult represents a matching session from full-text search.
type SearchResult struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Channel   string `json:"channel"`
	Label     string `json:"label"`
	Snippet   string `json:"snippet"`   // highlighted match context
	CreatedAt string `json:"created_at"`
	Score     float64 `json:"score"`
}

// SearchMessages performs full-text search across all session messages.
// Uses PostgreSQL's ts_vector/ts_query for fast text search.
// Returns matching sessions with highlighted snippets.
func (s *Store) SearchMessages(ctx context.Context, tenantID, query string, limit int) ([]SearchResult, error) {
	if query == "" || limit <= 0 {
		return nil, nil
	}
	if limit > 20 {
		limit = 20
	}

	// Convert user query to tsquery format
	tsQuery := toTsQuery(query)

	rows, err := s.pool.Query(ctx, `
		WITH msg_search AS (
			SELECT
				s.id AS session_id,
				s.agent_id,
				s.channel,
				s.label,
				s.created_at,
				msg->>'content' AS content,
				msg->>'role' AS role,
				ts_rank(
					to_tsvector('english', msg->>'content'),
					to_tsquery('english', $3)
				) AS rank
			FROM sessions s,
				jsonb_array_elements(s.messages) AS msg
			WHERE s.tenant_id = $1
				AND s.status != 'deleted'
				AND msg->>'content' IS NOT NULL
				AND msg->>'content' != ''
				AND to_tsvector('english', msg->>'content') @@ to_tsquery('english', $3)
		)
		SELECT DISTINCT ON (session_id)
			session_id,
			agent_id,
			channel,
			COALESCE(label, ''),
			ts_headline('english', content, to_tsquery('english', $3),
				'StartSel=>>>, StopSel=<<<, MaxWords=40, MinWords=20') AS snippet,
			created_at::text,
			rank
		FROM msg_search
		ORDER BY session_id, rank DESC
		LIMIT $2
	`, tenantID, limit, tsQuery)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.SessionID, &r.AgentID, &r.Channel, &r.Label, &r.Snippet, &r.CreatedAt, &r.Score); err != nil {
			continue
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListRecent returns metadata for the most recent sessions (no search, no LLM).
func (s *Store) ListRecent(ctx context.Context, tenantID string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, agent_id, channel, COALESCE(label, ''),
			COALESCE(
				(SELECT msg->>'content' FROM jsonb_array_elements(messages) AS msg
				 WHERE msg->>'role' = 'user' ORDER BY 1 DESC LIMIT 1),
				''
			) AS preview,
			created_at::text
		FROM sessions
		WHERE tenant_id = $1 AND status != 'deleted'
		ORDER BY updated_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent: %w", err)
	}
	defer rows.Close()

	results := []SearchResult{}
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.SessionID, &r.AgentID, &r.Channel, &r.Label, &r.Snippet, &r.CreatedAt); err != nil {
			continue
		}
		if len(r.Snippet) > 200 {
			r.Snippet = r.Snippet[:200] + "..."
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetSessionTranscript returns formatted conversation text for a session,
// truncated around query matches for LLM summarization.
func (s *Store) GetSessionTranscript(ctx context.Context, sessionID, query string, maxChars int) (string, error) {
	if maxChars <= 0 {
		maxChars = 100000
	}

	var messagesRaw json.RawMessage
	err := s.pool.QueryRow(ctx, `SELECT messages FROM sessions WHERE id = $1`, sessionID).Scan(&messagesRaw)
	if err != nil {
		return "", fmt.Errorf("get transcript: %w", err)
	}

	msgs := []Message{}
	if err := json.Unmarshal(messagesRaw, &msgs); err != nil {
		return "", fmt.Errorf("parse messages: %w", err)
	}

	// Format as readable transcript
	var b strings.Builder
	for _, msg := range msgs {
		if msg.Role == "system" {
			continue
		}
		role := strings.ToUpper(msg.Role)
		content := msg.Content
		// Truncate tool outputs
		if msg.Role == "tool" && len(content) > 500 {
			content = content[:250] + "\n...[truncated]...\n" + content[len(content)-250:]
		}
		b.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	text := b.String()
	if len(text) <= maxChars {
		return text, nil
	}

	// Centered truncation around query matches
	return truncateAroundQuery(text, query, maxChars), nil
}

// truncateAroundQuery centers the truncation window around the first query match.
func truncateAroundQuery(text, query string, maxChars int) string {
	if query == "" {
		// No query — take from start
		return text[:maxChars] + "\n...[truncated]..."
	}

	terms := strings.Fields(strings.ToLower(query))
	textLower := strings.ToLower(text)
	firstMatch := len(text)
	for _, term := range terms {
		if pos := strings.Index(textLower, term); pos >= 0 && pos < firstMatch {
			firstMatch = pos
		}
	}
	if firstMatch == len(text) {
		firstMatch = 0
	}

	half := maxChars / 2
	start := firstMatch - half
	if start < 0 {
		start = 0
	}
	end := start + maxChars
	if end > len(text) {
		end = len(text)
		start = end - maxChars
		if start < 0 {
			start = 0
		}
	}

	var result string
	if start > 0 {
		result = "...[earlier conversation truncated]...\n\n" + text[start:end]
	} else {
		result = text[start:end]
	}
	if end < len(text) {
		result += "\n\n...[later conversation truncated]..."
	}
	return result
}

// toTsQuery converts a user search string to PostgreSQL tsquery format.
func toTsQuery(query string) string {
	words := strings.Fields(strings.TrimSpace(query))
	if len(words) == 0 {
		return ""
	}
	// Join with & (AND) for multi-word queries
	escaped := make([]string, 0, len(words))
	for _, w := range words {
		// Remove special tsquery characters
		clean := strings.Map(func(r rune) rune {
			if r == '&' || r == '|' || r == '!' || r == '(' || r == ')' || r == ':' || r == '\'' {
				return -1
			}
			return r
		}, w)
		if clean != "" {
			escaped = append(escaped, clean)
		}
	}
	if len(escaped) == 0 {
		return ""
	}
	return strings.Join(escaped, " & ")
}
