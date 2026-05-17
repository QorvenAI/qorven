// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"

	"github.com/google/uuid"
)

type SkillInfo struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Path        string   `json:"path"`
	BaseDir     string   `json:"baseDir"`
	Source      string   `json:"source"`
	Description string   `json:"description"`
	Visibility  string   `json:"visibility,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Version     int      `json:"version,omitempty"`
	IsSystem    bool     `json:"is_system,omitempty"`
	Status      string   `json:"status,omitempty"`
	Enabled     bool     `json:"enabled"`
	Author      string   `json:"author,omitempty"`
	MissingDeps []string `json:"missing_deps,omitempty"`
}

type SkillSearchResult struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Path        string  `json:"path"`
	Score       float64 `json:"score"`
}

type SkillStore interface {
	ListSkills(ctx context.Context) []SkillInfo
	LoadSkill(ctx context.Context, name string) (string, bool)
	LoadForContext(ctx context.Context, allowList []string) string
	BuildSummary(ctx context.Context, allowList []string) string
	GetSkill(ctx context.Context, name string) (*SkillInfo, bool)
	FilterSkills(ctx context.Context, allowList []string) []SkillInfo
	Version() int64
	BumpVersion()
	Dirs() []string
}

type SkillAccessStore interface {
	ListAccessible(ctx context.Context, agentID uuid.UUID, userID string) ([]SkillInfo, error)
}

type EmbeddingSkillSearcher interface {
	SearchByEmbedding(ctx context.Context, embedding []float32, limit int) ([]SkillSearchResult, error)
	SetEmbeddingProvider(provider EmbeddingProvider)
	BackfillSkillEmbeddings(ctx context.Context) (int, error)
}

type SkillCreateParams struct {
	Name        string
	Slug        string
	Description *string
	OwnerID     string
	Visibility  string
	Status      string
	MissingDeps []string
	Version     int
	FilePath    string
	FileSize    int64
	FileHash    *string
	Frontmatter map[string]string
}

type SkillWithGrantStatus struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	Version     int       `json:"version"`
	Granted     bool      `json:"granted"`
	PinnedVer   *int      `json:"pinned_version,omitempty"`
	IsSystem    bool      `json:"is_system"`
}

type SkillManageStore interface {
	SkillStore
	CreateSkillManaged(ctx context.Context, p SkillCreateParams) (uuid.UUID, error)
	UpdateSkill(ctx context.Context, id uuid.UUID, updates map[string]any) error
	DeleteSkill(ctx context.Context, id uuid.UUID) error
	ToggleSkill(ctx context.Context, id uuid.UUID, enabled bool) error
	GetSkillByID(ctx context.Context, id uuid.UUID) (SkillInfo, bool)
	GetSkillOwnerID(ctx context.Context, id uuid.UUID) (string, bool)
	GetSkillOwnerIDBySlug(ctx context.Context, slug string) (string, bool)
	GetNextVersion(ctx context.Context, slug string) int
	GetNextVersionLocked(ctx context.Context, slug string) (int, func() error, error)
	IsSystemSkill(slug string) bool
	ListAllSkills(ctx context.Context) []SkillInfo
	ListAllSystemSkills(ctx context.Context) []SkillInfo
	ListSystemSkillDirs(ctx context.Context) map[string]string
	StoreMissingDeps(ctx context.Context, id uuid.UUID, missing []string) error
	GrantToAgent(ctx context.Context, skillID, agentID uuid.UUID, version int, grantedBy string) error
	RevokeFromAgent(ctx context.Context, skillID, agentID uuid.UUID) error
	GrantToUser(ctx context.Context, skillID uuid.UUID, userID, grantedBy string) error
	RevokeFromUser(ctx context.Context, skillID uuid.UUID, userID string) error
	ListWithGrantStatus(ctx context.Context, agentID uuid.UUID) ([]SkillWithGrantStatus, error)
	GetSkillFilePath(ctx context.Context, id uuid.UUID) (filePath string, slug string, version int, isSystem bool, ok bool)
}
