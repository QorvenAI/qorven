// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ConfigTypeFileWriter = "file_writer"
	ConfigTypeHeartbeat  = "heartbeat"
)

type ConfigPermission struct {
	ID         uuid.UUID       `json:"id"`
	AgentID    uuid.UUID       `json:"agentId"`
	Scope      string          `json:"scope"`
	ConfigType string          `json:"configType"`
	UserID     string          `json:"userId"`
	Permission string          `json:"permission"`
	GrantedBy  *string         `json:"grantedBy,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
}

type ConfigPermissionStore interface {
	CheckPermission(ctx context.Context, agentID uuid.UUID, scope, configType, userID string) (bool, error)
	Grant(ctx context.Context, perm *ConfigPermission) error
	Revoke(ctx context.Context, agentID uuid.UUID, scope, configType, userID string) error
	List(ctx context.Context, agentID uuid.UUID, configType, scope string) ([]ConfigPermission, error)
	ListFileWriters(ctx context.Context, agentID uuid.UUID, scope string) ([]ConfigPermission, error)
}

func CheckFileWriterPermission(ctx context.Context, permStore ConfigPermissionStore) error {
	if permStore == nil { return nil }
	userID := UserIDFromContext(ctx)
	if !strings.HasPrefix(userID, "group:") && !strings.HasPrefix(userID, "guild:") { return nil }
	agentID := AgentIDFromContext(ctx)
	if agentID == uuid.Nil { return nil }
	senderID := SenderIDFromContext(ctx)
	if senderID == "" { return nil }
	numericID := strings.SplitN(senderID, "|", 2)[0]
	allowed, err := permStore.CheckPermission(ctx, agentID, userID, ConfigTypeFileWriter, numericID)
	if err != nil { return nil }
	if !allowed {
		return fmt.Errorf("permission denied: only file writers can modify files in this group")
	}
	return nil
}
