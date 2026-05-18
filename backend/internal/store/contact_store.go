// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ChannelContact represents a user discovered through channel interactions.
type ChannelContact struct {
	ID              uuid.UUID  `json:"id"`
	ChannelType     string     `json:"channel_type"`
	ChannelInstance *string    `json:"channel_instance,omitempty"`
	SenderID        string     `json:"sender_id"`
	UserID          *string    `json:"user_id,omitempty"`
	DisplayName     *string    `json:"display_name,omitempty"`
	Username        *string    `json:"username,omitempty"`
	AvatarURL       *string    `json:"avatar_url,omitempty"`
	PeerKind        *string    `json:"peer_kind,omitempty"`
	ContactType     string     `json:"contact_type"`
	ThreadID        *string    `json:"thread_id,omitempty"`
	ThreadType      *string    `json:"thread_type,omitempty"`
	MergedID        *uuid.UUID `json:"merged_id,omitempty"`
	FirstSeenAt     time.Time  `json:"first_seen_at"`
	LastSeenAt      time.Time  `json:"last_seen_at"`
}

// ContactListOpts holds pagination and filter options for listing contacts.
type ContactListOpts struct {
	Search      string
	ChannelType string
	PeerKind    string
	ContactType string
	Limit       int
	Offset      int
}

// ContactStore manages channel contacts.
type ContactStore interface {
	UpsertContact(ctx context.Context, channelType, channelInstance, senderID, userID, displayName, username, peerKind, contactType, threadID, threadType string) error
	ListContacts(ctx context.Context, opts ContactListOpts) ([]ChannelContact, error)
	CountContacts(ctx context.Context, opts ContactListOpts) (int, error)
	GetContactsBySenderIDs(ctx context.Context, senderIDs []string) (map[string]ChannelContact, error)
	GetContactByID(ctx context.Context, id uuid.UUID) (*ChannelContact, error)
	GetSenderIDsByContactIDs(ctx context.Context, contactIDs []uuid.UUID) ([]string, error)
	MergeContacts(ctx context.Context, contactIDs []uuid.UUID, tenantUserID uuid.UUID) error
	UnmergeContacts(ctx context.Context, contactIDs []uuid.UUID) error
	GetContactsByMergedID(ctx context.Context, mergedID uuid.UUID) ([]ChannelContact, error)
	ResolveTenantUserID(ctx context.Context, channelType, senderID string) (string, error)
}
