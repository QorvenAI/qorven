// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/qorvenai/qorven/internal/approvalsx"
	"github.com/qorvenai/qorven/internal/workitems"
)

// OpenApproval persists a unified approval and reaches the user via the
// escalation ladder (risk → urgency). If the approval blocks a work item,
// the caller should also SetBlockedOn that work item to the approval id.
// Returns the approval id.
func (gw *Gateway) OpenApproval(ctx context.Context, userID string, a approvalsx.Approval) (string, error) {
	if gw.fabricApprovals == nil {
		return "", fmt.Errorf("unified approvals not available")
	}
	id, err := gw.fabricApprovals.Open(ctx, a)
	if err != nil {
		return "", err
	}
	body := a.Summary
	if _, rerr := gw.ReachUser(ctx, userID, "approval", id, "Approval needed", body, a.Risk); rerr != nil {
		slog.Warn("approvalsx.reach_user.failed", "approval", id, "err", rerr)
	}
	return id, nil
}

// DecideApproval records the decision, stops the escalation climb, and unblocks
// any work item that was waiting on this approval.
func (gw *Gateway) DecideApproval(ctx context.Context, id string, approved bool, decidedBy, note string) error {
	if err := gw.fabricApprovals.Decide(ctx, id, approved, decidedBy, note); err != nil {
		return err
	}
	if gw.reach != nil {
		_ = gw.reach.Ack(ctx, "approval", id)
	}
	// Unblock the work item that was blocked on this approval, if any.
	a, err := gw.fabricApprovals.Get(ctx, id)
	if err == nil && a.WorkItemID != "" && gw.workItems != nil {
		if approved {
			_ = gw.workItems.Unblock(ctx, a.WorkItemID, decidedBy)
		} else if err := gw.workItems.Transition(ctx, a.WorkItemID, workitems.StatusCancelled, decidedBy, "approval rejected"); err != nil {
			slog.Warn("approvalsx.decide.cancel_work_item_failed", "work_item_id", a.WorkItemID, "err", err)
		}
	}
	return nil
}
