// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"

	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/rooms"
	"github.com/qorvenai/qorven/internal/workitems"
)

// gatewayAgentRunner adapts agent.Loop.Run to the rooms.AgentRunner interface.
type gatewayAgentRunner struct{ gw *Gateway }

func (r *gatewayAgentRunner) Run(ctx context.Context, agentID, task string) (string, error) {
	res, err := r.gw.agentLoop.Run(ctx, agent.RunRequest{
		AgentID: agentID, UserMessage: task, Channel: "room", TenantID: defaultTenant,
	}, nil)
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.Content, nil
}

// buildDelegationOrchestrator wires the rooms delegation orchestrator to the
// gateway's real stores + agent loop.
func (gw *Gateway) buildDelegationOrchestrator() *rooms.Orchestrator {
	return &rooms.Orchestrator{
		Runner: &gatewayAgentRunner{gw: gw},
		Subordinates: func(ctx context.Context, headID string) ([]*agent.Agent, error) {
			return agent.GetSubordinates(ctx, gw.db.Pool, headID)
		},
		CreateWorkItem: func(ctx context.Context, ownerID, origin, requestedBy, title string) (string, error) {
			return gw.workItems.Create(ctx, workitems.WorkItem{
				TenantID: defaultTenant, Title: title, Origin: origin,
				OwnerAgentID: ownerID, RequestedBy: requestedBy, Status: workitems.StatusAssigned,
			})
		},
		TransitionWorkItem: func(ctx context.Context, id, to, actor, detail string) error {
			return gw.workItems.Transition(ctx, id, to, actor, detail)
		},
		PostRoom: func(ctx context.Context, roomID, senderID, senderType, content string) {
			gw.db.Pool.Exec(ctx,
				`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1,$2,$3,$4)`,
				roomID, senderID, senderType, content)
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": roomID, "sender": senderID, "content": content}})
			}
		},
		PostHub: func(ctx context.Context, content string) bool {
			hubID := gw.ensureCompanyHub(ctx)
			if hubID == "" {
				return false
			}
			gw.db.Pool.Exec(ctx,
				`INSERT INTO room_messages (room_id, sender_id, sender_type, content) VALUES ($1,'system','system',$2)`,
				hubID, content)
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{Type: "room_message", Data: map[string]string{"room_id": hubID, "sender": "system", "content": content}})
			}
			return true
		},
		RunHeadRollup: func(ctx context.Context, headID, prompt string) (string, error) {
			// Summary-only: ChatStream runs WITHOUT tools, so the head cannot
			// delegate again from the roll-up — this is the termination guard.
			return gw.agentLoop.ChatStream(ctx, headID, prompt, func(string) {})
		},
		BudgetOK: func(ctx context.Context, roomID string) bool {
			if gw.roomBudget == nil {
				return true
			}
			n, _ := gw.roomBudget.TurnsInWindow(ctx, roomID, rooms.DefaultWindow)
			return rooms.BudgetAllows(n, rooms.DefaultTurnCap)
		},
		RecordTurn: func(ctx context.Context, roomID, agentID string) {
			if gw.roomBudget != nil {
				_ = gw.roomBudget.RecordTurn(ctx, defaultTenant, roomID, agentID)
			}
		},
	}
}

// ensureCompanyHub returns the company-hub room id, creating it once if missing.
// A transaction advisory lock serializes concurrent creators so two roll-ups
// can't both insert a duplicate hub room (the rooms table has no unique name).
func (gw *Gateway) ensureCompanyHub(ctx context.Context) string {
	// Fast path: already exists.
	var id string
	if err := gw.db.Pool.QueryRow(ctx,
		`SELECT id FROM rooms WHERE tenant_id=$1 AND name='company-hub' LIMIT 1`, defaultTenant).Scan(&id); err == nil && id != "" {
		return id
	}
	// Slow path: lock, re-check, create.
	tx, err := gw.db.Pool.Begin(ctx)
	if err != nil {
		return ""
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "company-hub:"+defaultTenant); err != nil {
		return ""
	}
	// Re-check under the lock — another goroutine may have created it.
	if err := tx.QueryRow(ctx,
		`SELECT id FROM rooms WHERE tenant_id=$1 AND name='company-hub' LIMIT 1`, defaultTenant).Scan(&id); err == nil && id != "" {
		_ = tx.Commit(ctx)
		return id
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO rooms (tenant_id, name, display_name, description, created_by)
		 VALUES ($1,'company-hub','Company Hub','Company-wide coordination and roll-ups','system')
		 RETURNING id`, defaultTenant).Scan(&id); err != nil {
		return ""
	}
	if err := tx.Commit(ctx); err != nil {
		return ""
	}
	return id
}
