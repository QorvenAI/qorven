// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package training

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FeedbackRecord is a single feedback entry from the DB.
type FeedbackRecord struct {
	ID            string `json:"id"`
	AgentID       string `json:"agent_id"`
	SessionID     string `json:"session_id"`
	UserMessage   string `json:"user_message"`
	AgentResponse string `json:"agent_response"`
	Correction    string `json:"correction"`
	FeedbackType  string `json:"feedback_type"` // "like", "dislike", "superlike", "correction"
}

// TrainingExample is a single training row in JSONL format (OpenAI fine-tune compatible).
type TrainingExample struct {
	Messages []TrainingMessage `json:"messages"`
}

type TrainingMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PreferencePair is a DPO training pair (chosen vs rejected).
type PreferencePair struct {
	Prompt   string `json:"prompt"`
	Chosen   string `json:"chosen"`
	Rejected string `json:"rejected"`
}

// Exporter exports feedback data for fine-tuning.
type Exporter struct {
	pool *pgxpool.Pool
}

func NewExporter(pool *pgxpool.Pool) *Exporter { return &Exporter{pool: pool} }

// ExportJSONL exports positive feedback as JSONL training data.
func (e *Exporter) ExportJSONL(ctx context.Context, agentID string) ([]byte, error) {
	rows, err := e.pool.Query(ctx,
		`SELECT user_message, agent_response FROM feedback
		 WHERE agent_id = $1 AND feedback_type IN ('like', 'superlike')
		   AND user_message IS NOT NULL AND agent_response IS NOT NULL
		 ORDER BY created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []byte
	for rows.Next() {
		var userMsg, agentResp string
		if rows.Scan(&userMsg, &agentResp) != nil {
			continue
		}
		ex := TrainingExample{
			Messages: []TrainingMessage{
				{Role: "user", Content: userMsg},
				{Role: "assistant", Content: agentResp},
			},
		}
		line, _ := json.Marshal(ex)
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}

// ExportPreferences exports DPO preference pairs from like/dislike feedback.
func (e *Exporter) ExportPreferences(ctx context.Context, agentID string) ([]PreferencePair, error) {
	// Find prompts that have both liked and disliked responses
	rows, err := e.pool.Query(ctx,
		`SELECT f1.user_message, f1.agent_response AS chosen, f2.agent_response AS rejected
		 FROM feedback f1
		 JOIN feedback f2 ON f1.user_message = f2.user_message AND f1.agent_id = f2.agent_id
		 WHERE f1.agent_id = $1
		   AND f1.feedback_type IN ('like', 'superlike')
		   AND f2.feedback_type = 'dislike'
		   AND f1.id != f2.id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	pairs := []PreferencePair{}
	for rows.Next() {
		var p PreferencePair
		if rows.Scan(&p.Prompt, &p.Chosen, &p.Rejected) != nil {
			continue
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// ExportCorrections exports correction feedback as training data.
func (e *Exporter) ExportCorrections(ctx context.Context, agentID string) ([]byte, error) {
	rows, err := e.pool.Query(ctx,
		`SELECT agent_response, correction FROM feedback
		 WHERE agent_id = $1 AND feedback_type = 'correction'
		   AND correction IS NOT NULL AND correction != ''
		 ORDER BY created_at`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []byte
	for rows.Next() {
		var original, corrected string
		if rows.Scan(&original, &corrected) != nil {
			continue
		}
		ex := TrainingExample{
			Messages: []TrainingMessage{
				{Role: "user", Content: fmt.Sprintf("Improve this response:\n\n%s", original)},
				{Role: "assistant", Content: corrected},
			},
		}
		line, _ := json.Marshal(ex)
		out = append(out, line...)
		out = append(out, '\n')
	}
	return out, nil
}
