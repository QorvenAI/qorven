// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package channels

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const pairingCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0,O,1,I,L
const pairingCodeLen = 8
const pairingTTL = 60 * time.Minute

// PolicyChecker handles DM/group access control with pairing support.
type PolicyChecker struct {
	pool *pgxpool.Pool
}

func NewPolicyChecker(pool *pgxpool.Pool) *PolicyChecker {
	return &PolicyChecker{pool: pool}
}

// CheckDM returns (allowed bool, replyMsg string). If not allowed, replyMsg is sent back.
func (pc *PolicyChecker) CheckDM(ctx context.Context, tenantID, channelType, senderID, senderName string, policy string, allowlist []string) (bool, string) {
	switch policy {
	case "open", "":
		return true, ""
	case "disabled":
		return false, ""
	case "allowlist":
		for _, id := range allowlist {
			if id == senderID {
				return true, ""
			}
		}
		return false, "⛔ This bot is restricted to approved users only."
	case "pairing":
		// Check if already paired
		var count int
		pc.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM paired_devices WHERE tenant_id = $1 AND channel = $2 AND sender_id = $3`,
			tenantID, channelType, senderID).Scan(&count)
		if count > 0 {
			return true, ""
		}
		// Check allowlist too
		for _, id := range allowlist {
			if id == senderID {
				return true, ""
			}
		}
		// Generate pairing code
		code := pc.generateOrGetCode(ctx, tenantID, channelType, senderID, senderName)
		slog.Info("pairing.code_generated", "channel", channelType, "sender", senderID, "code", code)
		return false, fmt.Sprintf("🔐 Pairing required\n\nYour code: **%s**\n\nSend this code to your admin to get approved. Code expires in 60 minutes.", code)
	}
	return true, ""
}

func (pc *PolicyChecker) generateOrGetCode(ctx context.Context, tenantID, channelType, senderID, senderName string) string {
	// Check for existing pending code
	var existing string
	pc.pool.QueryRow(ctx,
		`SELECT pairing_code FROM pairing_requests WHERE tenant_id = $1 AND channel_type = $2 AND sender_id = $3 AND status = 'pending' AND expires_at > now()`,
		tenantID, channelType, senderID).Scan(&existing)
	if existing != "" {
		return existing
	}

	// Generate new code — populate both legacy (code/channel/chat_id) and new (pairing_code/channel_type) columns
	code := generateCode()
	pc.pool.Exec(ctx,
		`INSERT INTO pairing_requests (tenant_id, channel_type, channel, sender_id, sender_name, pairing_code, code, chat_id, expires_at)
		 VALUES ($1, $2, $2, $3, $4, $5, $5, $3, $6)`,
		tenantID, channelType, senderID, senderName, code, time.Now().Add(pairingTTL))
	return code
}

// ApprovePairing approves a pairing request by code.
func (pc *PolicyChecker) ApprovePairing(ctx context.Context, tenantID, code string) error {
	var channelType, senderID, senderName string
	err := pc.pool.QueryRow(ctx,
		`UPDATE pairing_requests SET status = 'approved' WHERE tenant_id = $1 AND pairing_code = $2 AND status = 'pending' AND expires_at > now()
		 RETURNING channel_type, sender_id, sender_name`,
		tenantID, code).Scan(&channelType, &senderID, &senderName)
	if err != nil {
		return fmt.Errorf("invalid or expired code")
	}
	// Add to paired devices
	pc.pool.Exec(ctx,
		`INSERT INTO paired_devices (tenant_id, channel, sender_id, chat_id, sender_name) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
		tenantID, channelType, senderID, senderID, senderName)
	slog.Info("pairing.approved", "channel", channelType, "sender", senderID, "name", senderName)
	return nil
}

// ListPending returns pending pairing requests.
func (pc *PolicyChecker) ListPending(ctx context.Context, tenantID string) ([]map[string]any, error) {
	rows, err := pc.pool.Query(ctx,
		`SELECT id, channel_type, sender_id, sender_name, pairing_code, expires_at, created_at
		 FROM pairing_requests WHERE tenant_id = $1 AND status = 'pending' AND expires_at > now() ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id, chType, senderID, senderName, code string
		var expiresAt, createdAt time.Time
		rows.Scan(&id, &chType, &senderID, &senderName, &code, &expiresAt, &createdAt)
		list = append(list, map[string]any{
			"id": id, "channel_type": chType, "sender_id": senderID, "sender_name": senderName,
			"pairing_code": code, "expires_at": expiresAt, "created_at": createdAt,
		})
	}
	return list, nil
}

func generateCode() string {
	b := make([]byte, pairingCodeLen)
	rand.Read(b)
	code := make([]byte, pairingCodeLen)
	for i := range code {
		code[i] = pairingCodeAlphabet[int(b[i])%len(pairingCodeAlphabet)]
	}
	return string(code)
}
