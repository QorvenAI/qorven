// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UsageSnapshot struct {
	ID                uuid.UUID  `json:"id"`
	BucketHour        time.Time  `json:"bucket_hour"`
	AgentID           *uuid.UUID `json:"agent_id,omitempty"`
	Provider          string     `json:"provider"`
	Model             string     `json:"model"`
	Channel           string     `json:"channel"`
	InputTokens       int64      `json:"input_tokens"`
	OutputTokens      int64      `json:"output_tokens"`
	CacheReadTokens   int64      `json:"cache_read_tokens"`
	CacheCreateTokens int64      `json:"cache_create_tokens"`
	ThinkingTokens    int64      `json:"thinking_tokens"`
	TotalCost         float64    `json:"total_cost"`
	RequestCount      int        `json:"request_count"`
	LLMCallCount      int        `json:"llm_call_count"`
	ToolCallCount     int        `json:"tool_call_count"`
	ErrorCount        int        `json:"error_count"`
	UniqueUsers       int        `json:"unique_users"`
	AvgDurationMS     int        `json:"avg_duration_ms"`
	MemoryDocs        int        `json:"memory_docs"`
	MemoryChunks      int        `json:"memory_chunks"`
	KGEntities        int        `json:"kg_entities"`
	KGRelations       int        `json:"kg_relations"`
	CreatedAt         time.Time  `json:"created_at"`
}

type SnapshotQuery struct {
	From     time.Time
	To       time.Time
	AgentID  *uuid.UUID
	Provider string
	Model    string
	Channel  string
	GroupBy  string
}

type SnapshotTimeSeries struct {
	BucketTime        time.Time `json:"bucket_time"`
	InputTokens       int64     `json:"input_tokens"`
	OutputTokens      int64     `json:"output_tokens"`
	CacheReadTokens   int64     `json:"cache_read_tokens"`
	CacheCreateTokens int64     `json:"cache_create_tokens"`
	ThinkingTokens    int64     `json:"thinking_tokens"`
	TotalCost         float64   `json:"total_cost"`
	RequestCount      int       `json:"request_count"`
	LLMCallCount      int       `json:"llm_call_count"`
	ToolCallCount     int       `json:"tool_call_count"`
	ErrorCount        int       `json:"error_count"`
	UniqueUsers       int       `json:"unique_users"`
	AvgDurationMS     int       `json:"avg_duration_ms"`
}

type SnapshotBreakdown struct {
	Key               string  `json:"key"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	CacheReadTokens   int64   `json:"cache_read_tokens"`
	CacheCreateTokens int64   `json:"cache_create_tokens"`
	TotalCost         float64 `json:"total_cost"`
	RequestCount      int     `json:"request_count"`
	LLMCallCount      int     `json:"llm_call_count"`
	ToolCallCount     int     `json:"tool_call_count"`
	ErrorCount        int     `json:"error_count"`
	AvgDurationMS     int     `json:"avg_duration_ms"`
}

type SnapshotStore interface {
	UpsertSnapshots(ctx context.Context, snapshots []UsageSnapshot) error
	GetTimeSeries(ctx context.Context, q SnapshotQuery) ([]SnapshotTimeSeries, error)
	GetBreakdown(ctx context.Context, q SnapshotQuery) ([]SnapshotBreakdown, error)
	GetLatestBucket(ctx context.Context) (*time.Time, error)
}
