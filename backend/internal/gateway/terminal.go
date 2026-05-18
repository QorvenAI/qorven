// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

// terminal.go — PTY-backed terminal sessions over WebSocket.
//
// Routes (all under /v1/terminal/sessions):
//   GET    /             list active sessions
//   POST   /             create a new session (spawns a shell)
//   DELETE /{id}         kill + remove a session
//   GET    /{id}/ws      WebSocket: bidirectional PTY I/O
//
// Each session owns one os.exec.Cmd + one PTY file descriptor. The WS
// handler pumps PTY output → WS and WS input → PTY. The session
// survives browser disconnect and reconnect (within the idle TTL).
//
// Wire protocol over the WebSocket:
//   Client → Server: {"type":"input","data":"ls -la\n"}
//                    {"type":"resize","cols":220,"rows":50}
//   Server → Client: {"type":"output","data":"<raw terminal bytes>"}
//                    {"type":"closed","code":0}
//
// The data field in output messages contains the raw PTY bytes (which
// include ANSI escape sequences). The frontend is expected to interpret
// them (xterm.js or similar) — or, as in the current simple pane,
// display them as plain text.

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

// ────────────────────────────────────────────────────────────
// In-memory session store
// ────────────────────────────────────────────────────────────

const (
	termIdleTTL    = 30 * time.Minute // idle sessions are reaped after this
	termMaxSessions = 20              // hard cap per server process
)

// TermSession is a running shell process behind a PTY.
type TermSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`

	mu      sync.Mutex
	ptmx    *os.File      // PTY master
	cmd     *exec.Cmd
	lastUse time.Time
	done    chan struct{}  // closed when the process exits
}

// TerminalStore holds all live terminal sessions.
type TerminalStore struct {
	mu       sync.Mutex
	sessions map[string]*TermSession
}

func newTerminalStore() *TerminalStore {
	ts := &TerminalStore{sessions: make(map[string]*TermSession)}
	go ts.reaper()
	return ts
}

func (ts *TerminalStore) add(sess *TermSession) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.sessions[sess.ID] = sess
}

func (ts *TerminalStore) get(id string) (*TermSession, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	s, ok := ts.sessions[id]
	return s, ok
}

func (ts *TerminalStore) remove(id string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if s, ok := ts.sessions[id]; ok {
		s.kill()
		delete(ts.sessions, id)
	}
}

func (ts *TerminalStore) list() []*TermSession {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	out := make([]*TermSession, 0, len(ts.sessions))
	for _, s := range ts.sessions {
		out = append(out, s)
	}
	return out
}

func (ts *TerminalStore) count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return len(ts.sessions)
}

func (ts *TerminalStore) reaper() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ts.mu.Lock()
		for id, s := range ts.sessions {
			s.mu.Lock()
			stale := time.Since(s.lastUse) > termIdleTTL
			s.mu.Unlock()
			if stale {
				s.kill()
				delete(ts.sessions, id)
				slog.Info("terminal.session.reaped", "id", id)
			}
		}
		ts.mu.Unlock()
	}
}

// kill terminates the shell and closes the PTY. Safe to call multiple times.
func (s *TermSession) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	if s.ptmx != nil {
		s.ptmx.Close()
	}
}

// ────────────────────────────────────────────────────────────
// HTTP handlers
// ────────────────────────────────────────────────────────────

func (gw *Gateway) handleListTerminalSessions(w http.ResponseWriter, r *http.Request) {
	if gw.termStore == nil {
		writeJSON(w, 200, []any{})
		return
	}
	sessions := gw.termStore.list()
	type row struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := make([]row, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, row{ID: s.ID, Name: s.Name, CreatedAt: s.CreatedAt})
	}
	writeJSON(w, 200, out)
}

func (gw *Gateway) handleCreateTerminalSession(w http.ResponseWriter, r *http.Request) {
	if gw.termStore == nil {
		writeJSON(w, 503, map[string]string{"error": "terminal service not available"})
		return
	}
	if gw.termStore.count() >= termMaxSessions {
		writeJSON(w, 429, map[string]string{"error": "max terminal sessions reached"})
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Name == "" {
		body.Name = fmt.Sprintf("Terminal %d", gw.termStore.count()+1)
	}

	sess, err := spawnSession(body.Name)
	if err != nil {
		slog.Error("terminal.spawn.failed", "error", err)
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	gw.termStore.add(sess)
	slog.Info("terminal.session.created", "id", sess.ID, "name", sess.Name)

	writeJSON(w, 200, map[string]any{
		"id":         sess.ID,
		"name":       sess.Name,
		"created_at": sess.CreatedAt,
	})
}

func (gw *Gateway) handleDeleteTerminalSession(w http.ResponseWriter, r *http.Request) {
	if gw.termStore == nil {
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	id := chi.URLParam(r, "id")
	gw.termStore.remove(id)
	writeJSON(w, 200, map[string]string{"status": "deleted", "id": id})
}

// handleTerminalWS upgrades to WebSocket and bridges browser ↔ PTY.
func (gw *Gateway) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	if gw.termStore == nil {
		http.Error(w, `{"error":"terminal service not available"}`, http.StatusServiceUnavailable)
		return
	}
	id := chi.URLParam(r, "id")
	sess, ok := gw.termStore.get(id)
	if !ok {
		http.Error(w, `{"error":"session not found"}`, 404)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Origin already gated by AuthMiddlewareV2
	})
	if err != nil {
		slog.Warn("terminal.ws.accept_failed", "error", err)
		return
	}
	defer conn.CloseNow()

	sess.mu.Lock()
	sess.lastUse = time.Now()
	ptmx := sess.ptmx
	sess.mu.Unlock()

	if ptmx == nil {
		conn.Close(websocket.StatusInternalError, "pty not initialized")
		return
	}

	ctx := conn.CloseRead(context.Background())

	// PTY → WS: pump output from the shell to the browser.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				msg, _ := json.Marshal(map[string]string{
					"type": "output",
					"data": string(buf[:n]),
				})
				if werr := conn.Write(ctx, websocket.MessageText, msg); werr != nil {
					return
				}
			}
			if err != nil {
				// PTY closed — process exited.
				exitCode := 0
				if sess.cmd != nil && sess.cmd.ProcessState != nil {
					exitCode = sess.cmd.ProcessState.ExitCode()
				}
				msg, _ := json.Marshal(map[string]any{
					"type": "closed",
					"code": exitCode,
				})
				conn.Write(ctx, websocket.MessageText, msg)
				return
			}
		}
	}()

	// WS → PTY: relay keystrokes and resize events from the browser.
	type clientMsg struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var msg clientMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		sess.mu.Lock()
		sess.lastUse = time.Now()
		sess.mu.Unlock()

		switch msg.Type {
		case "input":
			io.WriteString(ptmx, msg.Data)
		case "resize":
			if msg.Cols > 0 && msg.Rows > 0 {
				pty.Setsize(ptmx, &pty.Winsize{Cols: msg.Cols, Rows: msg.Rows})
			}
		}
	}
}

// ────────────────────────────────────────────────────────────
// Session spawner
// ────────────────────────────────────────────────────────────

func spawnSession(name string) (*TermSession, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
		if _, err := os.Stat(shell); err != nil {
			shell = "/bin/sh"
		}
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("pty.Start: %w", err)
	}

	sess := &TermSession{
		ID:        uuid.New().String(),
		Name:      name,
		CreatedAt: time.Now(),
		ptmx:      ptmx,
		cmd:       cmd,
		lastUse:   time.Now(),
		done:      make(chan struct{}),
	}

	// Watch for process exit so we can clean up.
	go func() {
		cmd.Wait()
		close(sess.done)
	}()

	return sess, nil
}
