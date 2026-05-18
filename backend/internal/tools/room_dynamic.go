// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package tools

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RoomManager provides dynamic room operations for MsgHub-style orchestration.
type RoomManager struct {
	pool *pgxpool.Pool
}

func NewRoomManager(pool *pgxpool.Pool) *RoomManager { return &RoomManager{pool: pool} }

func (rm *RoomManager) JoinRoom(ctx context.Context, roomID, agentID string) error {
	_, err := rm.pool.Exec(ctx,
		`INSERT INTO room_members (room_id, agent_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, roomID, agentID)
	return err
}

func (rm *RoomManager) LeaveRoom(ctx context.Context, roomID, agentID string) error {
	_, err := rm.pool.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1 AND agent_id = $2`, roomID, agentID)
	return err
}

func (rm *RoomManager) ListMembers(ctx context.Context, roomID string) ([]string, error) {
	rows, err := rm.pool.Query(ctx,
		`SELECT a.agent_key FROM room_members rm JOIN agents a ON rm.agent_id = a.id WHERE rm.room_id = $1`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		rows.Scan(&k)
		keys = append(keys, k)
	}
	return keys, nil
}

// --- Join Room Tool ---

type JoinRoomTool struct{ rm *RoomManager }

func NewJoinRoomTool(rm *RoomManager) *JoinRoomTool { return &JoinRoomTool{rm: rm} }
func (t *JoinRoomTool) Name() string                { return "join_room" }
func (t *JoinRoomTool) Description() string {
	return "Join a room/channel to participate in the conversation. Use when your skills are needed in a discussion."
}
func (t *JoinRoomTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_id": map[string]any{"type": "string", "description": "Room ID to join"},
		},
		"required": []string{"room_id"},
	}
}
func (t *JoinRoomTool) Execute(ctx context.Context, args map[string]any) *Result {
	roomID, _ := args["room_id"].(string)
	agentID := AgentIDFromCtx(ctx)
	if roomID == "" || agentID == "" {
		return ErrorResult("room_id required")
	}
	if err := t.rm.JoinRoom(ctx, roomID, agentID); err != nil {
		return ErrorResult("failed to join room: " + err.Error())
	}
	return TextResult(fmt.Sprintf("✅ Joined room %s", roomID))
}

// --- Leave Room Tool ---

type LeaveRoomTool struct{ rm *RoomManager }

func NewLeaveRoomTool(rm *RoomManager) *LeaveRoomTool { return &LeaveRoomTool{rm: rm} }
func (t *LeaveRoomTool) Name() string                 { return "leave_room" }
func (t *LeaveRoomTool) Description() string {
	return "Leave a room/channel when your participation is no longer needed."
}
func (t *LeaveRoomTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_id": map[string]any{"type": "string", "description": "Room ID to leave"},
		},
		"required": []string{"room_id"},
	}
}
func (t *LeaveRoomTool) Execute(ctx context.Context, args map[string]any) *Result {
	roomID, _ := args["room_id"].(string)
	agentID := AgentIDFromCtx(ctx)
	if roomID == "" || agentID == "" {
		return ErrorResult("room_id required")
	}
	if err := t.rm.LeaveRoom(ctx, roomID, agentID); err != nil {
		return ErrorResult("failed to leave room: " + err.Error())
	}
	return TextResult(fmt.Sprintf("👋 Left room %s", roomID))
}
