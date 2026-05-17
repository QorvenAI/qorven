// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"
	"time"
)

// BuiltinToolDef represents a built-in tool definition in the database.
type BuiltinToolDef struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Enabled     bool            `json:"enabled"`
	Settings    json.RawMessage `json:"settings"`
	Requires    []string        `json:"requires,omitempty"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// BuiltinToolStore manages built-in tool definitions.
type BuiltinToolStore interface {
	List(ctx context.Context) ([]BuiltinToolDef, error)
	Get(ctx context.Context, name string) (*BuiltinToolDef, error)
	Update(ctx context.Context, name string, updates map[string]any) error
	Seed(ctx context.Context, tools []BuiltinToolDef) error
	ListEnabled(ctx context.Context) ([]BuiltinToolDef, error)
	GetSettings(ctx context.Context, name string) (json.RawMessage, error)
}
