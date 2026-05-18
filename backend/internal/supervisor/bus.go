// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Intent markers — the structured protocol between agents.
// Every inter-agent message MUST contain exactly one intent.
// ACK is terminal — no reply allowed.
type Intent string

const (
	IntentStatusRequest    Intent = "STATUS_REQUEST"    // Prime → Qor: "What's your status?"
	IntentReviewRequest    Intent = "REVIEW_REQUEST"    // Qor → Prime: "I did X, please verify"
	IntentACK              Intent = "ACK"               // Any → Any: "Confirmed. Done." TERMINAL.
	IntentEscalationNotice Intent = "ESCALATION_NOTICE" // Prime → Human: "I can't resolve this."
	IntentAutoFix          Intent = "AUTO_FIX"          // Prime → Qor: "Apply this fix" (low-risk)
	IntentHeartbeat        Intent = "HEARTBEAT"         // Qor → Prime: "I'm alive." Informational.
)

// RiskLevel classifies the severity of an issue or proposed change.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"    // Auto-fixable by Prime (restart cron, retry API, clear cache)
	RiskMedium RiskLevel = "medium" // Prime proposes, human approves (adjust threshold, change model)
	RiskHigh   RiskLevel = "high"   // Prime escalates, human must approve (delete data, change creds)
)

// Message is a structured inter-agent communication.
// Every message has exactly one intent, one sender, one recipient.
type Message struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`      // agent ID (or "prime" or "human")
	To        string         `json:"to"`        // agent ID (or "prime" or "human")
	Intent    Intent         `json:"intent"`
	Content   string         `json:"content"`   // human-readable description
	Context   map[string]any `json:"context"`   // structured data (metrics, errors, proposed fix)
	Risk      RiskLevel      `json:"risk"`
	Timestamp time.Time      `json:"timestamp"`
	ReplyTo   string         `json:"reply_to"`  // ID of message being replied to (empty for new exchange)
	ExchangeID string       `json:"exchange_id"` // groups messages in the same conversation

	// Sync mode: nil = async (60s timeout), non-nil = sync with deadline
	SyncTimeout *time.Duration `json:"sync_timeout,omitempty"`
}

// Exchange tracks a single conversation between two agents.
// Max depth: 3 messages. ACK terminates immediately.
type Exchange struct {
	ID        string    `json:"id"`
	AgentA    string    `json:"agent_a"`    // initiator
	AgentB    string    `json:"agent_b"`    // responder
	Messages  []Message `json:"messages"`
	Status    string    `json:"status"`     // "open", "acked", "escalated", "timeout"
	StartedAt time.Time `json:"started_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}

// MaxExchangeDepth is the absolute maximum messages in one exchange.
// STATUS_REQUEST → REVIEW_REQUEST → ACK = 3 messages max.
const MaxExchangeDepth = 3

// Bus is the inter-agent message bus.
// It enforces the protocol rules: one intent per message, ACK is terminal,
// max depth per exchange, timeout handling.
type Bus struct {
	mu        sync.RWMutex
	exchanges map[string]*Exchange       // exchangeID → exchange
	inboxes   map[string]chan Message     // agentID → incoming messages
	handlers  map[string]MessageHandler   // agentID → handler function
	auditLog  []Message                   // all messages ever sent (for UI)
	onEscalation func(Message)            // callback when human needs to act
	onMessage    func(Message)            // called after every message (for persistence)
}

// MessageHandler processes an incoming inter-agent message and returns a response.
// Return nil to not respond (e.g., for ACK or HEARTBEAT).
type MessageHandler func(ctx context.Context, msg Message) *Message

// NewBus creates a new inter-agent message bus.
func NewBus(onEscalation func(Message)) *Bus {
	return &Bus{
		exchanges:    make(map[string]*Exchange),
		inboxes:      make(map[string]chan Message),
		handlers:     make(map[string]MessageHandler),
		onEscalation: onEscalation,
	}
}

// SetOnMessage sets a callback for every message sent (used for DB persistence).
func (b *Bus) SetOnMessage(fn func(Message)) { b.onMessage = fn }

// Register registers an agent's message handler on the bus.
func (b *Bus) Register(agentID string, handler MessageHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[agentID] = handler
	b.inboxes[agentID] = make(chan Message, 100)
}

// Send sends a message through the bus, enforcing all protocol rules.
func (b *Bus) Send(ctx context.Context, msg Message) error {
	// Assign ID and timestamp
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	msg.Timestamp = time.Now()

	// Validate: must have intent
	if msg.Intent == "" {
		return fmt.Errorf("protocol violation: message has no intent marker")
	}

	// Validate: must have sender and recipient
	if msg.From == "" || msg.To == "" {
		return fmt.Errorf("protocol violation: message must have from and to")
	}

	b.mu.Lock()

	// Record in audit log
	b.auditLog = append(b.auditLog, msg)

	// Persistence hook
	if b.onMessage != nil {
		go b.onMessage(msg)
	}

	// Check exchange depth
	if msg.ExchangeID != "" {
		ex, ok := b.exchanges[msg.ExchangeID]
		if ok {
			// Check if exchange is already closed
			if ex.Status == "acked" || ex.Status == "escalated" || ex.Status == "timeout" {
				b.mu.Unlock()
				return fmt.Errorf("protocol violation: exchange %s is already closed (%s)", msg.ExchangeID, ex.Status)
			}

			// Check max depth
			if len(ex.Messages) >= MaxExchangeDepth {
				b.mu.Unlock()
				return fmt.Errorf("protocol violation: exchange %s exceeded max depth (%d)", msg.ExchangeID, MaxExchangeDepth)
			}

			// Append message
			ex.Messages = append(ex.Messages, msg)

			// ACK terminates the exchange
			if msg.Intent == IntentACK {
				ex.Status = "acked"
				now := time.Now()
				ex.ClosedAt = &now
				slog.Info("supervisor.exchange.acked",
					"exchange", msg.ExchangeID,
					"from", msg.From,
					"depth", len(ex.Messages),
					"duration_ms", time.Since(ex.StartedAt).Milliseconds())
			}

			// Escalation closes the exchange
			if msg.Intent == IntentEscalationNotice {
				ex.Status = "escalated"
				now := time.Now()
				ex.ClosedAt = &now
			}
		}
	} else {
		// New exchange
		msg.ExchangeID = uuid.New().String()
		b.exchanges[msg.ExchangeID] = &Exchange{
			ID:        msg.ExchangeID,
			AgentA:    msg.From,
			AgentB:    msg.To,
			Messages:  []Message{msg},
			Status:    "open",
			StartedAt: time.Now(),
		}
	}

	b.mu.Unlock()

	// Route to recipient
	if msg.To == "human" && b.onEscalation != nil {
		b.onEscalation(msg)
		return nil
	}

	// Deliver to handler
	b.mu.RLock()
	handler, ok := b.handlers[msg.To]
	b.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for agent %s", msg.To)
	}

	// ACK and HEARTBEAT don't expect responses
	if msg.Intent == IntentACK {
		return nil // terminal — do not deliver, do not process
	}

	// HEARTBEAT is informational — deliver but don't expect response
	if msg.Intent == IntentHeartbeat {
		go handler(ctx, msg)
		return nil
	}

	// For all other intents, deliver and optionally wait for response
	if msg.SyncTimeout != nil {
		// Synchronous: block until response or timeout
		respCh := make(chan *Message, 1)
		go func() {
			resp := handler(ctx, msg)
			respCh <- resp
		}()

		select {
		case resp := <-respCh:
			if resp != nil {
				resp.ReplyTo = msg.ID
				resp.ExchangeID = msg.ExchangeID
				return b.Send(ctx, *resp)
			}
		case <-time.After(*msg.SyncTimeout):
			// Timeout — auto-ACK with warning
			b.mu.Lock()
			if ex, ok := b.exchanges[msg.ExchangeID]; ok {
				ex.Status = "timeout"
				now := time.Now()
				ex.ClosedAt = &now
			}
			b.mu.Unlock()
			slog.Warn("supervisor.exchange.timeout", "exchange", msg.ExchangeID, "to", msg.To)
		}
	} else {
		// Asynchronous: fire and forget, handler responds via Send()
		go func() {
			resp := handler(ctx, msg)
			if resp != nil {
				resp.ReplyTo = msg.ID
				resp.ExchangeID = msg.ExchangeID
				if err := b.Send(context.Background(), *resp); err != nil {
					slog.Error("supervisor.response.error", "error", err, "exchange", msg.ExchangeID)
				}
			}
		}()
	}

	return nil
}

// GetExchange returns an exchange by ID.
func (b *Bus) GetExchange(id string) *Exchange {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.exchanges[id]
}

// OpenExchanges returns all exchanges that haven't been closed.
func (b *Bus) OpenExchanges() []*Exchange {
	b.mu.RLock()
	defer b.mu.RUnlock()
	open := []*Exchange{}
	for _, ex := range b.exchanges {
		if ex.Status == "open" {
			open = append(open, ex)
		}
	}
	return open
}

// AuditLog returns the last N messages from the audit log.
func (b *Bus) AuditLog(limit int) []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if limit <= 0 || limit > len(b.auditLog) {
		limit = len(b.auditLog)
	}
	start := len(b.auditLog) - limit
	if start < 0 {
		start = 0
	}
	result := make([]Message, limit)
	copy(result, b.auditLog[start:])
	return result
}

// PendingEscalations returns all escalation messages awaiting human action.
func (b *Bus) PendingEscalations() []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	pending := []Message{}
	for _, msg := range b.auditLog {
		if msg.Intent == IntentEscalationNotice && msg.To == "human" {
			// Check if this escalation has been resolved
			resolved := false
			for _, m := range b.auditLog {
				if m.ReplyTo == msg.ID && m.Intent == IntentACK {
					resolved = true
					break
				}
			}
			if !resolved {
				pending = append(pending, msg)
			}
		}
	}
	return pending
}

// Stats returns bus statistics.
func (b *Bus) Stats() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var open, acked, escalated, timeout int
	for _, ex := range b.exchanges {
		switch ex.Status {
		case "open":
			open++
		case "acked":
			acked++
		case "escalated":
			escalated++
		case "timeout":
			timeout++
		}
	}

	return map[string]any{
		"total_exchanges":    len(b.exchanges),
		"open_exchanges":     open,
		"acked_exchanges":    acked,
		"escalated_exchanges": escalated,
		"timeout_exchanges":  timeout,
		"total_messages":     len(b.auditLog),
		"pending_escalations": len(b.PendingEscalations()),
	}
}

// JSON returns the stats as JSON.
func (b *Bus) JSON() []byte {
	data, _ := json.Marshal(b.Stats())
	return data
}

// RestoreFromDB reloads pending escalations from the DB store on startup.
// This closes Gap 4: supervisor review queue survives backend restarts.
// Must be called after all agents have registered their handlers.
func (b *Bus) RestoreFromDB(ctx context.Context, store *Store) error {
	if store == nil {
		return nil
	}
	pending, err := store.LoadPendingEscalations(ctx)
	if err != nil {
		return fmt.Errorf("supervisor.restore: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	restored := 0
	for _, msg := range pending {
		// Re-populate the exchange if missing (it was lost on restart)
		if _, exists := b.exchanges[msg.ExchangeID]; !exists {
			b.exchanges[msg.ExchangeID] = &Exchange{
				ID:        msg.ExchangeID,
				AgentA:    msg.From,
				AgentB:    msg.To,
				Status:    "open",
				StartedAt: msg.Timestamp,
				Messages:  []Message{msg},
			}
		} else {
			b.exchanges[msg.ExchangeID].Messages = append(b.exchanges[msg.ExchangeID].Messages, msg)
		}

		// Re-deliver to escalation handler so it can notify the supervisor agent
		if b.onEscalation != nil {
			go b.onEscalation(msg)
		}
		restored++
	}

	slog.Info("supervisor.bus.restored", "pending_escalations", restored)
	return nil
}
