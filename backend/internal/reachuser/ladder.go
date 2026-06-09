// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// Package reachuser implements the reach-the-user engine: a presence+time
// escalation ladder (in-app → IM → email) that reaches the human like a
// colleague would, where a single acknowledgement cancels the rest.
package reachuser

// Channel identifies which delivery adapter a rung uses.
const (
	ChannelInApp = "in_app"
	ChannelIM    = "im"
	ChannelEmail = "email"
)

// Wait windows between rungs (seconds). Defaults per spec.
const (
	waitInAppToIM = 300  // 5 min
	waitIMToEmail = 1800 // 30 min
)

// Input is everything the pure ladder needs to decide the next action.
type Input struct {
	Urgency     string // "low" | "normal" | "urgent"
	CurrentRung int    // 0 = nothing delivered yet; 1 in-app; 2 im; 3 email
	Online      bool   // is the user currently reachable in-app
}

// Decision is what the engine should do next.
type Decision struct {
	Done        bool   // no further delivery
	DeliverRung int    // the rung to deliver now (1..3); 0 when Done
	Channel     string // ChannelInApp | ChannelIM | ChannelEmail
	WaitSeconds int    // schedule next advance after this many seconds; 0 = no further advance OR (urgent) advance immediately
}

// Decide returns the next action for an escalation. Pure: no I/O, no clock.
//   - low: deliver in-app once, never escalate.
//   - normal: in-app (wait 5m) → IM (wait 30m) → email (done).
//   - urgent: same rungs but every WaitSeconds is 0 so the ticker climbs immediately.
func Decide(in Input) Decision {
	switch in.Urgency {
	case "low":
		if in.CurrentRung < 1 {
			return Decision{DeliverRung: 1, Channel: ChannelInApp, WaitSeconds: 0}
		}
		return Decision{Done: true}
	case "urgent":
		switch in.CurrentRung {
		case 0:
			return Decision{DeliverRung: 1, Channel: ChannelInApp, WaitSeconds: 0}
		case 1:
			return Decision{DeliverRung: 2, Channel: ChannelIM, WaitSeconds: 0}
		case 2:
			return Decision{DeliverRung: 3, Channel: ChannelEmail, WaitSeconds: 0}
		default:
			return Decision{Done: true}
		}
	default: // "normal" and any unknown urgency
		switch in.CurrentRung {
		case 0:
			return Decision{DeliverRung: 1, Channel: ChannelInApp, WaitSeconds: waitInAppToIM}
		case 1:
			return Decision{DeliverRung: 2, Channel: ChannelIM, WaitSeconds: waitIMToEmail}
		case 2:
			return Decision{DeliverRung: 3, Channel: ChannelEmail, WaitSeconds: 0}
		default:
			return Decision{Done: true}
		}
	}
}
