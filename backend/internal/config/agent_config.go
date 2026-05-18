// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package config

// ToolPolicySpec configures per-agent tool access policy.
type ToolPolicySpec struct {
	ToolCallPrefix string   `json:"toolCallPrefix,omitempty" toml:"tool_call_prefix"`
	AllowedTools   []string `json:"allowedTools,omitempty" toml:"allowed_tools"`
	DeniedTools    []string `json:"deniedTools,omitempty" toml:"denied_tools"`
}

// CompactionConfig controls session history compaction.
type CompactionConfig struct {
	MaxHistoryShare  float64 `json:"maxHistoryShare,omitempty" toml:"max_history_share"`
	KeepLastMessages int     `json:"keepLastMessages,omitempty" toml:"keep_last_messages"`
	MemoryFlush      *MemoryFlushConfig `json:"memoryFlush,omitempty" toml:"memory_flush"`
}

// MemoryFlushConfig controls when to flush memories from session history.
type MemoryFlushConfig struct {
	Enabled          bool    `json:"enabled" toml:"enabled"`
	ThresholdShare   float64 `json:"thresholdShare,omitempty" toml:"threshold_share"`
	MinCompactions   int     `json:"minCompactions,omitempty" toml:"min_compactions"`
}

// ContextPruningConfig controls trimming old tool results to save context window.
type ContextPruningConfig struct {
	SoftTrim  *ContextPruningSoftTrim  `json:"softTrim,omitempty" toml:"soft_trim"`
	HardClear *ContextPruningHardClear `json:"hardClear,omitempty" toml:"hard_clear"`
}

// ContextPruningSoftTrim truncates old tool results beyond a threshold.
type ContextPruningSoftTrim struct {
	Enabled       bool `json:"enabled" toml:"enabled"`
	MaxChars      int  `json:"maxChars,omitempty" toml:"max_chars"`
	KeepLastTurns int  `json:"keepLastTurns,omitempty" toml:"keep_last_turns"`
}

// ContextPruningHardClear removes old tool results entirely.
type ContextPruningHardClear struct {
	Enabled       bool `json:"enabled" toml:"enabled"`
	KeepLastTurns int  `json:"keepLastTurns,omitempty" toml:"keep_last_turns"`
}

// MemoryConfig controls per-agent memory behavior.
type MemoryConfig struct {
	EmbeddingModel   string `json:"embeddingModel,omitempty" toml:"embedding_model"`
	EmbeddingAPIBase string `json:"embeddingApiBase,omitempty" toml:"embedding_api_base"`
	MaxChunkLen      int    `json:"maxChunkLen,omitempty" toml:"max_chunk_len"`
	ChunkOverlap     int    `json:"chunkOverlap,omitempty" toml:"chunk_overlap"`
	SharedMemory     bool   `json:"sharedMemory,omitempty" toml:"shared_memory"`
	SharedKG         bool   `json:"sharedKg,omitempty" toml:"shared_kg"`
}

// SandboxConfig controls per-agent sandbox execution.
type SandboxConfig struct {
	Enabled         bool     `json:"enabled" toml:"enabled"`
	ContainerImage  string   `json:"containerImage,omitempty" toml:"container_image"`
	WorkspaceAccess string   `json:"workspaceAccess,omitempty" toml:"workspace_access"`
	DenyGroups      []string `json:"denyGroups,omitempty" toml:"deny_groups"`
}

// SubagentsConfig controls per-agent subagent/delegation behavior.
type SubagentsConfig struct {
	MaxConcurrent      int    `json:"maxConcurrent,omitempty" toml:"max_concurrent"`
	MaxSpawnDepth      int    `json:"maxSpawnDepth,omitempty" toml:"max_spawn_depth"`
	MaxChildrenPerAgent int   `json:"maxChildrenPerAgent,omitempty" toml:"max_children_per_agent"`
	ArchiveAfterMinutes int   `json:"archiveAfterMinutes,omitempty" toml:"archive_after_minutes"`
	Model              string `json:"model,omitempty" toml:"model"`
}

// ModelPricing holds cost information for a model.
type ModelPricing struct {
	InputPer1K  float64 `json:"inputPer1k" toml:"input_per_1k"`
	OutputPer1K float64 `json:"outputPer1k" toml:"output_per_1k"`
	CachePer1K  float64 `json:"cachePer1k,omitempty" toml:"cache_per_1k"`
}

// DefaultHistoryShare is the default fraction of context window used for history.
const DefaultHistoryShare = 0.6

// DefaultContextWindow is the fallback context window when not configured.
const DefaultContextWindow = 128000
