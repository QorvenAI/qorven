package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// BridgeProcess manages the Node.js Baileys sidecar for one WhatsApp channel.
// It is the WS *server*; the sidecar is the WS *client*.
type BridgeProcess struct {
	channelID  string
	dataDir    string
	sidecarBin string // path to compiled sidecar index.js; auto-resolved if empty

	mu        sync.RWMutex
	clients   map[*websocket.Conn]struct{}
	qrSubs    []func(string)
	msgSubs   []func(BridgeMessage)
	statusSub func(string, string)

	httpSrv  *http.Server
	cmd      *exec.Cmd
	port     string
	upgrader websocket.Upgrader
}

// BridgeMessage is an inbound message event from the sidecar.
type BridgeMessage struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	FromName string `json:"from_name"`
	Chat     string `json:"chat"`
	Body     string `json:"body"`
	TS       int64  `json:"ts"`
}

// NewBridgeProcess creates a BridgeProcess. sidecarBin may be empty to auto-resolve.
func NewBridgeProcess(channelID, dataDir, sidecarBin string) *BridgeProcess {
	bp := &BridgeProcess{
		channelID:  channelID,
		dataDir:    dataDir,
		sidecarBin: sidecarBin,
		clients:    make(map[*websocket.Conn]struct{}),
	}
	bp.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			return host == "127.0.0.1" || host == "::1"
		},
	}
	return bp
}

// StartServer starts the WS server on a random localhost port.
// Returns the ":PORT" string. Does NOT spawn the sidecar.
func (bp *BridgeProcess) StartServer(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("bridge listen: %w", err)
	}
	bp.port = fmt.Sprintf(":%d", ln.Addr().(*net.TCPAddr).Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", bp.handleWS)
	bp.httpSrv = &http.Server{Handler: mux}
	go bp.httpSrv.Serve(ln)

	go func() {
		<-ctx.Done()
		bp.httpSrv.Close()
	}()

	return bp.port, nil
}

// SpawnSidecar starts the Node.js sidecar process.
func (bp *BridgeProcess) SpawnSidecar(ctx context.Context) error {
	bin := bp.sidecarBin
	if bin == "" {
		bin = bp.resolveSidecarBin()
	}
	if bin == "" {
		return fmt.Errorf("whatsapp sidecar not found — run 'make build-sidecar'")
	}

	wsURL := "ws://127.0.0.1" + bp.port

	bp.cmd = exec.CommandContext(ctx, "node", bin)
	bp.cmd.Env = append(os.Environ(),
		"WS_URL="+wsURL,
		"INSTANCE_ID="+bp.channelID,
		"DATA_DIR="+bp.dataDir,
		"HEADLESS=false",
	)
	bp.cmd.Stdout = os.Stdout
	bp.cmd.Stderr = os.Stderr

	if err := bp.cmd.Start(); err != nil {
		return fmt.Errorf("spawn sidecar: %w", err)
	}
	slog.Info("whatsapp.sidecar.started", "channel", bp.channelID, "pid", bp.cmd.Process.Pid)

	go func() {
		bp.cmd.Wait()
		slog.Info("whatsapp.sidecar.exited", "channel", bp.channelID)
	}()

	return nil
}

func (bp *BridgeProcess) resolveSidecarBin() string {
	self, _ := os.Executable()
	candidates := []string{
		filepath.Join(filepath.Dir(self), "sidecar", "whatsapp", "dist", "index.js"),
		filepath.Join(filepath.Dir(self), "..", "sidecar", "whatsapp", "dist", "index.js"),
		"sidecar/whatsapp/dist/index.js",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// SubscribeQR registers a callback for QR code events (base64 PNG data URL).
func (bp *BridgeProcess) SubscribeQR(fn func(string)) {
	bp.mu.Lock()
	bp.qrSubs = append(bp.qrSubs, fn)
	bp.mu.Unlock()
}

// SubscribeMessages registers a callback for inbound messages.
func (bp *BridgeProcess) SubscribeMessages(fn func(BridgeMessage)) {
	bp.mu.Lock()
	bp.msgSubs = append(bp.msgSubs, fn)
	bp.mu.Unlock()
}

// SubscribeStatus registers a callback for connected/disconnected events.
func (bp *BridgeProcess) SubscribeStatus(fn func(status, reason string)) {
	bp.mu.Lock()
	bp.statusSub = fn
	bp.mu.Unlock()
}

// Send instructs the sidecar to send a message.
func (bp *BridgeProcess) Send(to, text string) error {
	return bp.broadcast(map[string]string{"type": "send", "to": to, "text": text})
}

// RequestQR instructs the sidecar to emit the latest QR.
func (bp *BridgeProcess) RequestQR() error {
	return bp.broadcast(map[string]string{"type": "request_qr"})
}

// RequestPairingCode asks the sidecar for a phone-number pairing code.
func (bp *BridgeProcess) RequestPairingCode(phone string) error {
	return bp.broadcast(map[string]string{"type": "request_pairing_code", "phone": phone})
}

func (bp *BridgeProcess) broadcast(msg any) error {
	payload, _ := json.Marshal(msg)
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	if len(bp.clients) == 0 {
		return fmt.Errorf("no sidecar connected")
	}
	for conn := range bp.clients {
		conn.WriteMessage(websocket.TextMessage, payload)
	}
	return nil
}

func (bp *BridgeProcess) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := bp.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("whatsapp.bridge.ws_upgrade_failed", "error", err)
		return
	}
	defer conn.Close()

	bp.mu.Lock()
	bp.clients[conn] = struct{}{}
	bp.mu.Unlock()
	defer func() {
		bp.mu.Lock()
		delete(bp.clients, conn)
		bp.mu.Unlock()
	}()

	slog.Info("whatsapp.bridge.sidecar_connected", "channel", bp.channelID)

	for {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var evt map[string]any
		if err := json.Unmarshal(raw, &evt); err != nil {
			continue
		}

		evtType, _ := evt["type"].(string)
		switch evtType {
		case "qr":
			qr, _ := evt["qr"].(string)
			bp.mu.RLock()
			for _, sub := range bp.qrSubs {
				go sub(qr)
			}
			bp.mu.RUnlock()

		case "connected":
			phone, _ := evt["phone"].(string)
			slog.Info("whatsapp.sidecar.connected", "phone", phone, "channel", bp.channelID)
			bp.mu.RLock()
			if bp.statusSub != nil {
				go bp.statusSub("connected", phone)
			}
			bp.mu.RUnlock()

		case "disconnected":
			reason, _ := evt["reason"].(string)
			slog.Warn("whatsapp.sidecar.disconnected", "reason", reason, "channel", bp.channelID)
			bp.mu.RLock()
			if bp.statusSub != nil {
				go bp.statusSub("disconnected", reason)
			}
			bp.mu.RUnlock()

		case "message":
			var msg BridgeMessage
			data, _ := json.Marshal(evt)
			if json.Unmarshal(data, &msg) == nil {
				bp.mu.RLock()
				for _, sub := range bp.msgSubs {
					go sub(msg)
				}
				bp.mu.RUnlock()
			}

		case "pairing_code":
			code, _ := evt["code"].(string)
			slog.Info("whatsapp.pairing_code", "code", code, "channel", bp.channelID)

		case "ping":
			conn.WriteJSON(map[string]string{"type": "pong"})
		}
	}
}

// Stop shuts down the WS server and kills the sidecar.
func (bp *BridgeProcess) Stop() {
	if bp.httpSrv != nil {
		bp.httpSrv.Close()
	}
	if bp.cmd != nil && bp.cmd.Process != nil {
		bp.cmd.Process.Kill()
	}
}
