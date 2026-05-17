// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PendingMessage represents a buffered group chat message.
type PendingMessage struct {
	ID            uuid.UUID `json:"id"`
	ChannelName   string    `json:"channel_name"`
	HistoryKey    string    `json:"history_key"`
	Sender        string    `json:"sender"`
	SenderID      string    `json:"sender_id"`
	Body          string    `json:"body"`
	PlatformMsgID string    `json:"platform_msg_id"`
	IsSummary     bool      `json:"is_summary"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PendingMessageGroup is a summary row for the grouped overview.
type PendingMessageGroup struct {
	ChannelName  string    `json:"channel_name"`
	HistoryKey   string    `json:"history_key"`
	GroupTitle   string    `json:"group_title,omitempty"`
	MessageCount int       `json:"message_count"`
	HasSummary   bool      `json:"has_summary"`
	LastActivity time.Time `json:"last_activity"`
}

// PendingMessageStore persists group chat messages for context when bot is mentioned.
type PendingMessageStore interface {
	AppendBatch(ctx context.Context, msgs []PendingMessage) error
	ListByKey(ctx context.Context, channelName, historyKey string) ([]PendingMessage, error)
	DeleteByKey(ctx context.Context, channelName, historyKey string) error
	Compact(ctx context.Context, deleteIDs []uuid.UUID, summary *PendingMessage) error
	DeleteStale(ctx context.Context, olderThan time.Duration) (int64, error)
	ListGroups(ctx context.Context) ([]PendingMessageGroup, error)
	CountAll(ctx context.Context) (int64, error)
	CountByKey(ctx context.Context, channelName, historyKey string) (int, error)
	ResolveGroupTitles(ctx context.Context, groups []PendingMessageGroup) (map[string]string, error)
}
