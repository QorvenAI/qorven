// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// Event types pushed to clients.
const (
	EventNewMessage     = "new_message"
	EventSoulCompleted  = "soul_completed"
	EventSoulActivity   = "soul_activity"
	EventTaskUpdated    = "task_updated"
	EventCronFired      = "cron_fired"
	EventSoulCreated    = "soul_created"
	EventNotification   = "notification"
	EventTicketUpdated  = "ticket_updated"
	EventTicketComment  = "ticket_comment"
	EventProjectUpdated = "project_updated"
	EventBudgetWarning  = "budget_warning"
	// EventServiceHealth is broadcast when a backend dependency (e.g. DB)
	// changes health state. Clients use it to show a specific degradation
	// message even while the WebSocket connection itself remains open.
	// Payload: { "database": "ok"|"unavailable", "status": "healthy"|"degraded" }
	EventServiceHealth = "service_health"

	// Autonomous runtime events (migration 071)
	EventRuntimeStateChanged = "runtime_state_changed"
	EventTaskIterationStart  = "task_iteration_start"
	EventTaskToolCall        = "task_tool_call"
	EventTaskProgress        = "task_progress"
	EventTaskDone            = "task_done"
	EventTaskBlocked         = "task_blocked"
	EventSynthesisTriggered  = "synthesis_triggered"
)

// Event is a real-time event pushed to all connected clients.
type Event struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	Data      any    `json:"data,omitempty"`
	Timestamp int64  `json:"timestamp"`
	Seq       int64  `json:"seq,omitempty"` // monotonic sequence — clients use this to detect and reorder out-of-order delivery
}

// presenceSetter is the subset of the presence.Store used by Hub.
// Keeping it as an interface avoids an import cycle and makes the Hub
// independently testable.
type presenceSetter interface {
	SetOnline(ctx context.Context, userID, tenantID, channel string) error
	SetOffline(ctx context.Context, userID string) error
}

// Client is a connected WebSocket client.
type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	hub      *Hub
	userID   string
	tenantID string
}

// Hub manages all WebSocket connections and broadcasts events.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	broadcast  chan Event
	register   chan *Client
	unregister chan *Client
	seqCounter atomic.Int64 // monotonic sequence number — survives hub restart
	presence   presenceSetter
}

// SetPresence wires a presence store into the hub. Call once during
// Gateway initialisation, before Run() is invoked.
func (h *Hub) SetPresence(p presenceSetter) { h.presence = p }

// NewHub creates a new real-time hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Event, 512), // increased from 256 to reduce silent drop risk
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// nextSeq returns the next monotonic sequence number.
func (h *Hub) nextSeq() int64 { return h.seqCounter.Add(1) }

// Run starts the hub event loop. Call in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			slog.Info("realtime.client.connected", "total", len(h.clients))
			if h.presence != nil && client.userID != "" {
				go h.presence.SetOnline(context.Background(), client.userID, client.tenantID, "web")
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Info("realtime.client.disconnected", "total", len(h.clients))
			if h.presence != nil && client.userID != "" {
				go h.presence.SetOffline(context.Background(), client.userID)
			}

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					// Client too slow, disconnect
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to ALL connected clients.
func (h *Hub) Broadcast(event Event) {
	event.Timestamp = time.Now().UnixMilli()
	event.Seq = h.nextSeq() // monotonic sequence for client-side ordering
	select {
	case h.broadcast <- event:
	default:
		// Buffer full — log with sequence so clients can detect the gap
		slog.Warn("realtime.broadcast.dropped", "type", event.Type, "seq", event.Seq,
			"hint", "client should re-fetch session history on reconnect to fill gap")
	}
}

// BroadcastNewMessage notifies all clients of a new chat message.
func (h *Hub) BroadcastNewMessage(sessionID, agentID, role, content string, extras ...string) {
	data := map[string]string{
		"role":    role,
		"content": content,
	}
	// Optional: channel, sender_name
	if len(extras) >= 1 && extras[0] != "" {
		data["channel"] = extras[0]
	}
	if len(extras) >= 2 && extras[1] != "" {
		data["sender_name"] = extras[1]
	}
	h.Broadcast(Event{
		Type:      EventNewMessage,
		SessionID: sessionID,
		AgentID:   agentID,
		Data:      data,
	})
}

// BroadcastSoulCompleted notifies that a Soul finished a delegated task.
func (h *Hub) BroadcastSoulCompleted(agentID, soulName, taskTitle, result string) {
	h.Broadcast(Event{
		Type:    EventSoulCompleted,
		AgentID: agentID,
		Data: map[string]string{
			"soul":   soulName,
			"task":   taskTitle,
			"result": result,
		},
	})
}

// BroadcastSoulActivity streams live Soul work progress to all clients.
// P3.1: Live Soul activity stream — unique feature, no other framework does this.
func (h *Hub) BroadcastSoulActivity(agentID, soulKey, status, detail string) {
	h.Broadcast(Event{
		Type:    EventSoulActivity,
		AgentID: agentID,
		Data: map[string]string{
			"soul_key": soulKey,
			"status":   status,
			"detail":   detail,
		},
	})
}

// BroadcastNotification sends a notification to all clients.
func (h *Hub) BroadcastNotification(title, body string) {
	h.Broadcast(Event{
		Type: EventNotification,
		Data: map[string]string{
			"title": title,
			"body":  body,
		},
	})
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// HandleWebSocket upgrades HTTP to WebSocket and registers the client.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		slog.Warn("realtime.ws.accept_failed", "error", err)
		return
	}

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 64),
		hub:      h,
		userID:   r.URL.Query().Get("user_id"),
		tenantID: r.URL.Query().Get("tenant_id"),
	}

	h.register <- client

	// Shared cancel so the writer, reader, and heartbeat goroutines all
	// tear down together when any one of them fails. Previously the
	// writer loop ran on `conn.Write` failure alone — a zombie reader
	// could keep the FD open indefinitely when the client's TCP stack
	// didn't notice a dead peer (common on mobile networks and behind
	// corporate NATs that silently drop idle flows).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Writer goroutine — sends events to this client
	go func() {
		defer func() {
			cancel()
			h.unregister <- client
			conn.Close(websocket.StatusNormalClosure, "")
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-client.send:
				if !ok {
					return
				}
				wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				wcancel()
				if err != nil {
					return
				}
			}
		}
	}()

	// Heartbeat — server-initiated ping every 20s with a 10s timeout.
	// A dead client's ping will fail long before the OS-level TCP
	// keep-alive (which defaults to 2 hours on Linux). Shared helper
	// lives in gateway/resilience.go — realtime runs without an import
	// cycle by copying the small constants here. The critical detail
	// is that the cancel() we pass cascades to the reader + writer
	// goroutines above so they all unwind together.
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					slog.Debug("realtime.ws.ping_failed", "user_id", client.userID, "error", err)
					cancel()
					return
				}
			}
		}
	}()

	// Reader goroutine — drains frames from the client (mostly pongs
	// and the occasional app-level message). Exits on any error, which
	// triggers the shared cancel.
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			cancel()
			return
		}
	}
}
