// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qorvenai/qorven/internal/agent"
	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/api/sessioncancel"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/byom"
	"github.com/qorvenai/qorven/internal/deployment"
	orchestratorpkg "github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/orchestrator/handlers"
	"github.com/qorvenai/qorven/internal/permissions"
	wasmregistry "github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/plugins/wasm"
	"github.com/qorvenai/qorven/internal/plans"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
)

// replicaCountFromEnv reads QORVEN_REPLICAS to feed the DraftStore guard.
// Default 1 (single binary). Bad values log and fall back to 1 — the
// guard's job is to catch intentional multi-replica setups, not a typo.
func replicaCountFromEnv() int {
	raw := os.Getenv("QORVEN_REPLICAS")
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		slog.Warn("invalid QORVEN_REPLICAS; defaulting to 1", "value", raw)
		return 1
	}
	return n
}

// submitHardTimeout caps a single agent turn; blocks runaway goroutines
// that survive an ssh reconnect or a crashed client. One cancel path
// runs when the caller clicks abort; this one runs if they just
// disappear. BYOM users tune this via QORVEN_SUBMIT_TIMEOUT — a local
// Llama 8B run may legitimately exceed the hosted-inference default.
func submitHardTimeout() time.Duration {
	return byom.Load().SubmitHardTimeout
}

// ensureProtocolSurfaces constructs the gateway-scoped event Emitter,
// cancel registry, service-account store, and Command Server,
// idempotently. Called from New() and from test setups before routes
// are mounted.
func (gw *Gateway) ensureProtocolSurfaces() {
	if gw.sessionCancels == nil {
		gw.sessionCancels = sessioncancel.NewRegistry()
	}
	if gw.events == nil {
		gw.events = apievents.NewEmitter(
			apievents.WithHub(gw.rtHub),
			apievents.WithLogger(slog.Default()),
		)
	}
	if gw.serviceAccounts == nil && gw.db != nil {
		gw.serviceAccounts = serviceaccounts.NewStore(gw.db.Pool)
		// Best-effort initial load; failures are logged and the first
		// Lookup retries. A persistent failure here is a config problem
		// (DB down) that other components will surface louder.
		if err := gw.serviceAccounts.Refresh(context.Background()); err != nil {
			slog.Warn("serviceaccounts: initial refresh failed", "err", err)
		}
	}
	if gw.plans == nil && gw.db != nil {
		gw.plans = plans.NewStore(gw.db.Pool)
	}
	if gw.approvals == nil && gw.db != nil {
		gw.approvals = approvals.NewStore(gw.db.Pool)
	}
	if gw.permissionGate == nil && gw.db != nil {
		gw.permissionGate = permissions.NewGate(gw.db.Pool, gw.events)
	}
	if gw.wasmPluginStore == nil && gw.db != nil {
		gw.wasmPluginStore = wasmregistry.NewStore(gw.db.Pool)
	}
	// Wasm host is a gateway-lifetime singleton.
	// ensureProtocolSurfaces can run multiple times — re-creating the
	// Host each call leaks compiled modules. Build once, cache on gw.
	if gw.wasmHost == nil {
		if host, err := wasm.NewHost(context.Background(), wasm.Config{}); err == nil {
			gw.wasmHost = host
		} else {
			slog.Warn("wasm host init failed; plugin upload works but loads won't",
				"err", err)
		}
	}
	if gw.wasmPluginLoader == nil && gw.wasmPluginStore != nil && gw.wasmHost != nil {
		gw.wasmPluginLoader = wasmregistry.NewLoader(
			gw.wasmPluginStore, gw.wasmHost,
			func() *permissions.Gate { return gw.permissionGate },
			slog.Default(),
		)
	}
	// per-tenant quota. Process-lifetime singleton; the
	// sweeper goroutine runs until the background context cancels
	// (i.e. process exit).
	if gw.tenantQuota == nil {
		gw.tenantQuota = NewTenantQuota(context.Background())
	}
	if gw.orchestrator == nil && gw.plans != nil && gw.agentLoop != nil {
		// pass the Wasm plugin loader as the tenant-tool
		// resolver. nil-safe — when the loader wasn't built (no Wasm
		// host) the orchestrator's handlers run unchanged.
		var resolver handlers.TenantToolResolver
		if gw.wasmPluginLoader != nil {
			resolver = gw.wasmPluginLoader
		}
		gw.orchestrator = orchestratorpkg.NewServiceWithTools(
			gw.plans, gw.approvals, gw.agentLoop, resolver,
			gw.events, slog.Default(),
		)
	}
	if gw.deploymentConfig == nil && gw.db != nil {
		gw.deploymentConfig = deployment.NewConfig(gw.db.Pool)
		// Best-effort initial Refresh; Mode() falls back to
		// single_tenant if the DB is unreachable so the gateway never
		// loses availability because of a config-read blip.
		if err := gw.deploymentConfig.Refresh(context.Background()); err != nil {
			slog.Warn("deployment: initial refresh failed", "err", err)
		}

		// multi-tenant installs MUST connect to
		// Postgres as a restricted role. A superuser bypasses RLS
		// regardless of policy/FORCE, turning the entire data-layer
		// boundary into decoration. Fail fast at boot if the role
		// doesn't meet the contract — the operator needs to fix
		// their DSN before anything else can proceed.
		if gw.deploymentConfig.IsMultiTenant(context.Background()) {
			if err := gw.db.AssertNotSuperuser(context.Background()); err != nil {
				slog.Error("multi-tenant boot guard tripped", "err", err)
				panic(fmt.Sprintf(
					"gateway: refusing to boot in multi-tenant mode under a "+
						"privileged DB role: %v", err))
			}
		}
	}
	if gw.cmdServer == nil {
		gw.cmdServer = &apicommands.Server{
			Emitter:    gw.events,
			Drafts:     apicommands.NewDraftStoreGuarded(30*time.Minute, replicaCountFromEnv()),
			Submit:     gw.protocolSubmit,
			Run:        gw.protocolRunCommand,
			Resolve:    gw.protocolResolveSession,
			OwnerCheck: gw.protocolOwnerCheck,
			Logger:     slog.Default(),
		}
	}
}

// protocolOwnerCheck is the SessionOwnerCheck wired into the command
// Server. Thin adapter over gateway.authorize — see authorize.go for
// the single-source-of-truth rules.
//
// this function kept as a named hook because the
// apicommands.Server type references it by signature; the body is
// now a one-liner delegating to the unified helper. Future security
// patches must go through gateway.authorize, not here.
func (gw *Gateway) protocolOwnerCheck(ctx context.Context, sessionID string) error {
	return gw.authorizeSessionID(ctx, sessionID)
}

// actorFromContext extracts the authenticated user id from the request
// context. Empty string when no auth is configured (local dev mode).
func actorFromContext(ctx context.Context) string {
	if u := userFromContext(ctx); u != nil {
		return u.ID
	}
	return ""
}

// protocolSubmit runs a single agent turn for the session. The user
// message is persisted synchronously before the goroutine launches so
// callers receive the canonical message id the message store assigned.
// The run's context is registered with sessionCancels so a subsequent
// "abort" command actually stops it.
func (gw *Gateway) protocolSubmit(ctx context.Context, sessionID, agentID, text string, meta map[string]string) (string, error) {
	if gw.agentLoop == nil {
		return "", errors.New("agent loop not configured")
	}
	if sessionID == "" {
		return "", errors.New("session_id required")
	}
	if strings.TrimSpace(text) == "" {
		return "", errors.New("empty prompt")
	}

	actor := actorFromContext(ctx)
	resolvedAgent := agentID
	if resolvedAgent == "" {
		resolvedAgent = "default"
	}

	channel := "web"
	if ch, ok := meta["channel"]; ok && ch != "" {
		channel = ch
	}

	// Persist the user message synchronously. We hold this id as the
	// canonical correlation key for the whole turn — streamed part.updated
	// events reference it, the response envelope returns it, and a later
	// GET /v1/sessions/<s>/messages/<id> resolves to the same row.
	//
	// We fall back to a synthesized id only when no message store is wired
	// (tests, minimal configs). In that case a slog warning records the
	// degradation.
	msgID, err := gw.persistUserMessage(ctx, sessionID, resolvedAgent, text)
	if err != nil {
		return "", fmt.Errorf("persist user message: %w", err)
	}

	// Register a cancel handle for this turn BEFORE launching the goroutine.
	// We intentionally do not use ctx as the parent — the HTTP request's
	// context will be cancelled the moment SubmitPrompt returns.
	hardTimeout := submitHardTimeout()
	runCtx, cancel := context.WithTimeout(context.Background(), hardTimeout)
	release := gw.sessionCancels.Register(sessionID, cancel, time.Now().Add(hardTimeout))

	req := agent.RunRequest{
		AgentID:     resolvedAgent,
		SessionID:   sessionID,
		UserMessage: text,
		Channel:     channel,
		Stream:      true,
		// NoPersist: the user message is already in the store; we want the
		// agent loop to skip the duplicate insert. The agent loop's own
		// assistant response is still persisted via its internal path.
		NoPersist:   true,
	}

	go func() {
		defer release()
		defer cancel()

		_, runErr := gw.agentLoop.Run(runCtx, req, func(ev agent.StreamEvent) {
			switch ev.Type {
			case "text_delta":
				if ev.Delta == "" {
					return
				}
				partID := fmt.Sprintf("%s-part-%d", msgID, time.Now().UnixNano())
				payload, _ := json.Marshal(map[string]string{"text": ev.Delta})
				_ = gw.events.Emit(runCtx, apievents.SinkAll, apievents.TypeMessagePartUpdated,
					apievents.MessagePartUpdatedProps{
						MessageID: msgID,
						PartID:    partID,
						Kind:      "text",
						Order:     0,
						Payload:   payload,
					})
			case "tool_start":
				_ = gw.events.Emit(runCtx, apievents.SinkAll, apievents.TypeAgentProgress,
					apievents.AgentProgressProps{
						AgentKey: resolvedAgent,
						Kind:     "tool_start",
						Detail:   map[string]any{"tool": ev.Tool, "args": ev.Args},
					})
			case "tool_result", "tool_end":
				_ = gw.events.Emit(runCtx, apievents.SinkAll, apievents.TypeAgentProgress,
					apievents.AgentProgressProps{
						AgentKey: resolvedAgent,
						Kind:     "tool_end",
						Detail:   map[string]any{"tool": ev.Tool, "ok": ev.Error == ""},
					})
			}
		})

		// Distinguish user-abort (context.Canceled via sessionCancels) from
		// system failure. runCtx.Err() is the most reliable signal: when we
		// cancel via the registry it returns context.Canceled; when the
		// hard timeout fires it returns context.DeadlineExceeded.
		switch {
		case runErr == nil:
			_ = gw.events.Emit(context.Background(), apievents.SinkAll,
				apievents.TypeSessionIdle,
				apievents.SessionIdleProps{SessionID: sessionID, Actor: actor})

		case errors.Is(runCtx.Err(), context.Canceled):
			// User or admin cancellation. Pull the tags recorded by
			// sessioncancel.Cancel; Lookup returns zero-Cancel tag fields.
			tags, _ := gw.sessionCancels.Lookup(sessionID)
			code := string(tags.Code)
			if code == "" {
				code = string(sessioncancel.CodeUserAbort)
			}
			_ = gw.events.Emit(context.Background(), apievents.SinkAll,
				apievents.TypeSessionCancelled,
				apievents.SessionCancelledProps{
					SessionID: sessionID,
					Actor:     tags.Actor,
					Reason:    tags.Reason,
					Code:      code,
				})

		case errors.Is(runCtx.Err(), context.DeadlineExceeded):
			_ = gw.events.Emit(context.Background(), apievents.SinkAll,
				apievents.TypeSessionCancelled,
				apievents.SessionCancelledProps{
					SessionID: sessionID,
					Actor:     "system",
					Reason:    fmt.Sprintf("turn exceeded %s", hardTimeout),
					Code:      string(sessioncancel.CodeTimeout),
				})

		default:
			_ = gw.events.Emit(context.Background(), apievents.SinkAll,
				apievents.TypeSessionError,
				apievents.SessionErrorProps{
					SessionID: sessionID,
					Message:   runErr.Error(),
					Severity:  "error",
				})
			// Still emit idle so the client re-enables input.
			_ = gw.events.Emit(context.Background(), apievents.SinkAll,
				apievents.TypeSessionIdle,
				apievents.SessionIdleProps{SessionID: sessionID, Actor: actor})
		}
	}()

	return msgID, nil
}

// persistUserMessage synchronously writes the user message to the session
// store and returns the canonical id. When the session store is not
// configured we fall back to a synthesized id and log a warning — tests
// and embedded deployments may run without a DB.
func (gw *Gateway) persistUserMessage(ctx context.Context, sessionID, agentID, text string) (string, error) {
	if gw.sessions == nil {
		id := fmt.Sprintf("msg-%d", time.Now().UnixNano())
		slog.Warn("commands.submit_prompt: no session store; using synthetic id",
			"msg_id", id, "session_id", sessionID)
		return id, nil
	}
	id, err := gw.sessions.AppendUserMessage(ctx, sessionID, agentID, text)
	if err != nil {
		return "", err
	}
	return id, nil
}

// protocolRunCommand handles POST /v1/commands/execute. Implemented
// commands: clear, new_session, toggle_sidebar, open_theme, abort.
func (gw *Gateway) protocolRunCommand(ctx context.Context, cmd string, rawArgs json.RawMessage) (map[string]any, error) {
	switch cmd {
	case "clear":
		var args struct {
			SessionID string `json:"session_id"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("bad args: %w", err)
			}
		}
		_ = gw.events.Emit(ctx, apievents.SinkAll, apievents.TypeSessionUpdated,
			apievents.SessionUpdatedProps{
				SessionID: args.SessionID,
				Changes:   map[string]any{"action": "clear"},
			})
		return map[string]any{"session_id": args.SessionID, "cleared": true}, nil

	case "new_session":
		var args struct {
			AgentID string `json:"agent_id"`
			Channel string `json:"channel"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("bad args: %w", err)
			}
		}
		if gw.sessions == nil {
			return nil, errors.New("session store not configured")
		}
		agentID := args.AgentID
		if agentID == "" {
			agentID = "default"
		}
		channel := args.Channel
		if channel == "" {
			channel = "web"
		}
		sess, err := gw.sessions.Create(ctx, defaultTenant, agentID, "operator", channel)
		if err != nil {
			return nil, fmt.Errorf("create session: %w", err)
		}
		_ = gw.events.Emit(ctx, apievents.SinkAll, apievents.TypeSessionCreated,
			apievents.SessionCreatedProps{
				SessionID: sess.ID,
				AgentID:   agentID,
				Channel:   channel,
			})
		return map[string]any{"session_id": sess.ID, "agent_id": agentID}, nil

	case "toggle_sidebar":
		_ = gw.events.Emit(ctx, apievents.SinkAll, apievents.TypeSessionUpdated,
			apievents.SessionUpdatedProps{Changes: map[string]any{"ui": "toggle_sidebar"}})
		return map[string]any{"ok": true}, nil

	case "open_theme":
		_ = gw.events.Emit(ctx, apievents.SinkAll, apievents.TypeSessionUpdated,
			apievents.SessionUpdatedProps{Changes: map[string]any{"open_picker": "themes"}})
		return map[string]any{"picker": "themes"}, nil

	case "abort":
		var args struct {
			SessionID string `json:"session_id"`
			Reason    string `json:"reason,omitempty"`
		}
		if len(rawArgs) > 0 {
			if err := json.Unmarshal(rawArgs, &args); err != nil {
				return nil, fmt.Errorf("bad args: %w", err)
			}
		}
		if args.SessionID == "" {
			return nil, errors.New("session_id required")
		}

		actor := actorFromContext(ctx)
		// Admin actors may abort any session; reflected in code. Phase 2
		// adds the ownership middleware, which will reject non-owner
		// non-admin abort attempts before reaching this handler.
		code := sessioncancel.CodeUserAbort
		if u := userFromContext(ctx); u != nil && u.Role == "admin" {
			code = sessioncancel.CodeAdminAbort
		}

		ok := gw.sessionCancels.Cancel(args.SessionID, code, actor, args.Reason)
		// We intentionally do NOT emit session.cancelled here — that event
		// is emitted by the goroutine on unwind, after the agent loop has
		// actually returned. That keeps the event timing honest: the
		// cancel flag flips first, then the goroutine drains, then the
		// "cancelled" signal reaches the client.
		return map[string]any{
			"session_id": args.SessionID,
			"aborted":    ok,
			"actor":      actor,
			"code":       string(code),
		}, nil
	}
	return nil, fmt.Errorf("unknown command %q", cmd)
}

// protocolResolveSession accepts either a session UUID or session_key and
// returns the canonical id. Phase 1 passes through; Phase 3 rewires this
// to consult sessions store when the schema splits land.
func (gw *Gateway) protocolResolveSession(_ context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", errors.New("empty session id")
	}
	return sessionID, nil
}
