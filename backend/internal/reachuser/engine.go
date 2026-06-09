// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package reachuser

import (
	"context"
	"time"
)

// Deliverer performs the actual delivery for one rung and reports presence.
// Implemented by the gateway (in-app notification, IM channel send, email).
type Deliverer interface {
	IsOnline(ctx context.Context, userID string) bool
	Deliver(ctx context.Context, e Escalation, rung int, channel string) error
}

// Engine drives escalations up the ladder. now() is injected for testability.
type Engine struct {
	store *Store
	deliv Deliverer
	now   func() time.Time
}

func NewEngine(store *Store, deliv Deliverer, now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{store: store, deliv: deliv, now: now}
}

// isLastRung reports whether `rung` is the final rung for this urgency.
// low urgency stops after the in-app rung (1); all others end at email (3).
func isLastRung(urgency string, rung int) bool {
	if urgency == "low" {
		return rung >= 1
	}
	return rung >= 3
}

// nextAdvance converts a just-made Decision into the absolute next_advance_at to
// persist. The urgency is passed explicitly (Decision intentionally does not carry it):
//   - WaitSeconds > 0            → now + wait (timed climb, e.g. normal 5m/30m)
//   - just-delivered rung is last → zero time (no further advance: low after in-app, any after email)
//   - otherwise                  → now (urgent: climb immediately on the next tick)
func nextAdvance(now time.Time, urgency string, d Decision) time.Time {
	if d.WaitSeconds > 0 {
		return now.Add(time.Duration(d.WaitSeconds) * time.Second)
	}
	if isLastRung(urgency, d.DeliverRung) {
		return time.Time{}
	}
	return now // urgent, more rungs remain → due immediately
}

// Open delivers rung 1 right away and persists the escalation with its next advance
// time (per the ladder). Returns the escalation id. Never blocks on the climb.
func (e *Engine) Open(ctx context.Context, esc Escalation) (string, error) {
	if esc.Urgency == "" {
		esc.Urgency = "normal"
	}
	online := e.deliv.IsOnline(ctx, esc.UserID)
	d := Decide(Input{Urgency: esc.Urgency, CurrentRung: 0, Online: online})
	esc.CurrentRung = d.DeliverRung
	id, err := e.store.Open(ctx, esc, nextAdvance(e.now(), esc.Urgency, d))
	if err != nil {
		return "", err
	}
	esc.ID = id
	// Deliver rung 1 now (best-effort; a delivery failure still records the escalation).
	if derr := e.deliv.Deliver(ctx, esc, d.DeliverRung, d.Channel); derr != nil {
		e.store.LogStep(ctx, id, d.DeliverRung, d.Channel, "failed", derr.Error())
	} else {
		e.store.LogStep(ctx, id, d.DeliverRung, d.Channel, "delivered", "")
	}
	return id, nil
}

// Tick advances every due pending escalation by one rung. Call from a ticker.
func (e *Engine) Tick(ctx context.Context) error {
	due, err := e.store.DuePending(ctx, e.now())
	if err != nil {
		return err
	}
	for _, esc := range due {
		d := Decide(Input{Urgency: esc.Urgency, CurrentRung: esc.CurrentRung, Online: e.deliv.IsOnline(ctx, esc.UserID)})
		if d.Done {
			e.store.Exhaust(ctx, esc.ID)
			continue
		}
		if derr := e.deliv.Deliver(ctx, esc, d.DeliverRung, d.Channel); derr != nil {
			e.store.LogStep(ctx, esc.ID, d.DeliverRung, d.Channel, "failed", derr.Error())
		} else {
			e.store.LogStep(ctx, esc.ID, d.DeliverRung, d.Channel, "delivered", "")
		}
		next := nextAdvance(e.now(), esc.Urgency, d)
		if next.IsZero() {
			// Last rung delivered with no further advance → record final rung, then finish.
			e.store.Advance(ctx, esc.ID, d.DeliverRung, time.Time{})
			e.store.Exhaust(ctx, esc.ID)
		} else {
			e.store.Advance(ctx, esc.ID, d.DeliverRung, next)
		}
	}
	return nil
}

// Ack marks the matching escalation acknowledged so the climb stops.
func (e *Engine) Ack(ctx context.Context, kind, refID string) error {
	return e.store.Ack(ctx, kind, refID)
}
