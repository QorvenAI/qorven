// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	"nhooyr.io/websocket"
)

// RPCRequest is a JSON-RPC style message from client.
type RPCRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// RPCResponse is sent back to client.
type RPCResponse struct {
	ID     string      `json:"id,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// WSClient represents a connected WebSocket client.
type WSClient struct {
	conn     *websocket.Conn
	tenantID string
	userID   string
	authed   bool
	mu       sync.Mutex
}

func (c *WSClient) Send(resp RPCResponse) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, _ := json.Marshal(resp)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.conn.Write(ctx, websocket.MessageText, data)
}

func (gw *Gateway) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		slog.Error("ws accept failed", "error", err)
		return
	}
	defer conn.CloseNow()

	client := &WSClient{conn: conn}
	slog.Info("ws client connected", "remote", r.RemoteAddr)

	// Cancellable context drives the heartbeat AND the read loop — if
	// the peer stops responding to pings we cancel, which causes the
	// Read below to return an error and we exit cleanly. Without this,
	// a client that crashes without closing its TCP socket would hold
	// this goroutine hostage for 2 hours (Linux TCP keepalive default).
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	runWSHeartbeat(ctx, cancel, conn, "ws-rpc")

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				slog.Debug("ws client disconnected", "status", websocket.CloseStatus(err))
			}
			return
		}

		var req RPCRequest
		if err := json.Unmarshal(data, &req); err != nil {
			client.Send(RPCResponse{Error: &RPCError{Code: -32700, Message: "parse error"}})
			continue
		}

		resp := gw.handleRPC(ctx, client, req)
		client.Send(resp)
	}
}

func (gw *Gateway) handleRPC(ctx context.Context, client *WSClient, req RPCRequest) RPCResponse {
	// Auth must be first message
	if !client.authed && req.Method != "auth" {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 401, Message: "authenticate first"}}
	}

	switch req.Method {
	case "auth":
		return gw.rpcAuth(ctx, client, req)
	case "ping":
		return RPCResponse{ID: req.ID, Result: map[string]string{"pong": time.Now().UTC().Format(time.RFC3339)}}
	case "chat.send":
		return gw.rpcChatSend(ctx, client, req)
	case "agents.list":
		return RPCResponse{ID: req.ID, Result: map[string]interface{}{"agents": []interface{}{}}}
	case "sessions.list":
		return RPCResponse{ID: req.ID, Result: map[string]interface{}{"sessions": []interface{}{}}}
	default:
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: -32601, Message: "method not found: " + req.Method}}
	}
}

func (gw *Gateway) rpcChatSend(ctx context.Context, client *WSClient, req RPCRequest) RPCResponse {
	if gw.agentLoop == nil {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 500, Message: "agent loop not initialized"}}
	}
	var params struct {
		AgentID   string `json:"agent_id"`
		SessionID string `json:"session_id"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.AgentID == "" || params.Message == "" {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 400, Message: "agent_id and message required"}}
	}

	// Stream events back to this WebSocket client
	result, err := gw.agentLoop.Run(ctx, agent.RunRequest{
		AgentID:     params.AgentID,
		SessionID:   params.SessionID,
		UserMessage: params.Message,
		Channel:     "web",
	}, func(event agent.StreamEvent) {
		client.Send(RPCResponse{Result: map[string]any{"type": event.Type, "data": event.Data}})
	})
	if err != nil {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 500, Message: err.Error()}}
	}
	return RPCResponse{ID: req.ID, Result: map[string]any{
		"content":    result.Content,
		"tools_used": result.ToolsUsed,
		"session_id": params.SessionID,
	}}
}

func (gw *Gateway) rpcAuth(ctx context.Context, client *WSClient, req RPCRequest) RPCResponse {
	var params struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 400, Message: "token required"}}
	}

	if gw.cfg.Auth.Token != "" && params.Token != gw.cfg.Auth.Token {
		return RPCResponse{ID: req.ID, Error: &RPCError{Code: 401, Message: "invalid token"}}
	}

	client.authed = true
	client.userID = "operator"
	slog.Info("ws client authenticated", "user", client.userID)
	return RPCResponse{ID: req.ID, Result: map[string]interface{}{
		"status": "authenticated",
		"role":   "operator",
	}}
}
