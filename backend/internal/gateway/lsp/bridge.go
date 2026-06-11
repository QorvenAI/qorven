// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.
package lsp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// writeFramed writes a Content-Length framed LSP message to w.
// The LSP wire format is:
//
//	Content-Length: <N>\r\n\r\n<json payload>
func writeFramed(w io.Writer, payload []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFramed reads one Content-Length framed LSP message from r and returns
// the raw JSON payload (without the header).
func readFramed(r *bufio.Reader) ([]byte, error) {
	var contentLength int
	// Read headers until blank line (\r\n\r\n).
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("lsp: reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			// Blank line signals end of headers.
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("lsp: bad Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
		// Ignore other headers (Content-Type etc.) — LSP rarely uses them.
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("lsp: missing or zero Content-Length")
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("lsp: reading body (%d bytes): %w", contentLength, err)
	}
	return buf, nil
}

// serverProc is a running language server process.
type serverProc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	lang   string
}

// Manager lazily spawns and (for v1) manages per-connection language server
// processes. A per-connection process model is used: a new server is spawned
// for each WebSocket connection and killed when the connection closes. This is
// the simplest correct implementation; idle-process reuse is a future
// optimisation.
type Manager struct {
	mu    sync.Mutex
	procs map[string]*serverProc // key = projectID + ":" + lang (unused in v1 per-connection model)
}

// NewManager creates a new LSP bridge manager.
func NewManager() *Manager {
	return &Manager{
		procs: make(map[string]*serverProc),
	}
}

// Spawn starts a new language server process for the given language/project
// root. The caller is responsible for calling Kill on the returned proc when
// done.
func (m *Manager) Spawn(projectID, lang, rootPath string) (*serverProc, error) {
	bin, args, ok := resolveServer(lang)
	if !ok {
		return nil, fmt.Errorf("no language server for %q", lang)
	}

	cmd := exec.Command(bin, args...)
	// Set the working directory to the project root so the server can find
	// workspace files without needing an explicit rootUri initialisation.
	if rootPath != "" {
		cmd.Dir = rootPath
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		return nil, fmt.Errorf("lsp: start %q: %w", bin, err)
	}

	slog.Info("lsp.server.started",
		"lang", lang, "bin", bin, "pid", cmd.Process.Pid,
		"project", projectID, "root", rootPath)

	return &serverProc{
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: bufio.NewReader(stdoutPipe),
		lang:   lang,
	}, nil
}

// kill terminates the server process. Safe to call multiple times.
func (p *serverProc) kill() {
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Wait() //nolint:errcheck — best-effort reap
	}
}

// Relay spawns a language server (per-connection, v1 design) and bridges the
// WebSocket ↔ stdio transports until ctx is cancelled or either side errors.
//
// Transport contract (confirmed correct for vscode-ws-jsonrpc / monaco-languageclient):
//   - WebSocket carries raw unframed JSON-RPC messages (one JSON object per WS frame).
//   - The language server's stdio carries Content-Length framed messages.
//   - The bridge therefore converts:
//     WS frame (raw JSON) → Content-Length framed stdin write
//     stdout Content-Length frame → raw JSON WS write
//
// The LSP initialize handshake is client-driven: monaco-languageclient sends
// "initialize" as the first message. The bridge passes it through transparently
// and never injects its own protocol messages.
func (m *Manager) Relay(
	ctx context.Context,
	projectID, lang, rootPath string,
	wsRead func() ([]byte, error),
	wsWrite func([]byte) error,
) error {
	proc, err := m.Spawn(projectID, lang, rootPath)
	if err != nil {
		return err
	}
	defer proc.kill()

	// errCh carries the first error from either pump goroutine.
	errCh := make(chan error, 2)

	// Pump 1: stdout (server) → WS (browser).
	// Reads Content-Length framed messages from the server and writes raw JSON
	// frames to the WebSocket.
	go func() {
		for {
			msg, err := readFramed(proc.stdout)
			if err != nil {
				errCh <- fmt.Errorf("lsp: stdout read: %w", err)
				return
			}
			if err := wsWrite(msg); err != nil {
				errCh <- fmt.Errorf("lsp: ws write: %w", err)
				return
			}
		}
	}()

	// Pump 2: WS (browser) → stdin (server).
	// Reads raw JSON frames from the WebSocket and writes Content-Length
	// framed messages to the server's stdin.
	go func() {
		for {
			// Check context before blocking read.
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			default:
			}
			msg, err := wsRead()
			if err != nil {
				errCh <- fmt.Errorf("lsp: ws read: %w", err)
				return
			}
			if err := writeFramed(proc.stdin, msg); err != nil {
				errCh <- fmt.Errorf("lsp: stdin write: %w", err)
				return
			}
		}
	}()

	// Wait for the first pump to fail or the context to cancel.
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
