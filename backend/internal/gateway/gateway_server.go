// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	cronpkg "github.com/qorvenai/qorven/internal/cron"
	daemonpkg "github.com/qorvenai/qorven/internal/daemon"
	qorvtls "github.com/qorvenai/qorven/internal/tls"
	orchestratorpkg "github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/bus"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/heartbeat"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/tools"
	"golang.org/x/crypto/acme/autocert"
)

func (gw *Gateway) findWebDir() string {
	// Explicit config override wins. Operators set
	// [server] web_dir = "/var/lib/qorven/web" after `pnpm build` to
	// ship a customised UI without rebuilding the binary.
	if gw.cfg != nil && gw.cfg.Server.WebDir != "" {
		d := gw.cfg.Server.WebDir
		if info, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(d)
			return abs
		}
		slog.Warn("web_dir configured but index.html missing — falling back", "web_dir", d)
	}
	exe, _ := os.Executable()
	home, _ := os.UserHomeDir()
	candidates := []string{
		"web",
		filepath.Join(filepath.Dir(exe), "web"),
		filepath.Join(home, ".qorven", "web"),
	}
	for _, d := range candidates {
		if info, err := os.Stat(filepath.Join(d, "index.html")); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(d)
			return abs
		}
	}
	return ""
}

func (gw *Gateway) Start() error {
	// Kill any stale qorven process from a previous run before we bind
	// ports or start channel pollers. Prevents the "two processes, one
	// Telegram bot" conflict that causes 409 errors and agent run failures.
	evictStalePID()

	// Bootstrap Chief of Staff agent
	if gw.agents != nil {
		if _, err := gw.ensureChief(context.Background()); err != nil {
			slog.Warn("chief.bootstrap_failed", "error", err)
		} else {
			slog.Info("chief of staff ready")
		}
	}

	// Start real-time WebSocket hub
	go gw.rtHub.Run()
	slog.Info("realtime hub started")

	// Start heartbeat worker for autonomous ticket processing
	if gw.db != nil && gw.agentLoop != nil {
		gw.startHeartbeatWorker(context.Background())
		slog.Info("heartbeat worker started")
	}

	// Start health monitor — probes DB and broadcasts service_health events
	// to connected clients on state transitions (e.g. DB goes down/recovers).
	if gw.db != nil {
		gw.startHealthMonitor(context.Background())
		slog.Info("health monitor started")
	}

	// Start background memory dreamer (consolidates memories on interval)
	if gw.dreamer != nil {
		go gw.dreamer.Start(context.Background())
		slog.Info("dreamer started")
	}

	// Start the plan-graph restart sweeper. Recovers plans whose
	// approval-to-execute goroutine died because the gateway was
	// restarted between approve and execute. Without this, a user's
	// approved plan would be silently abandoned. (Phase 2 gate item.)
	if gw.orchestrator != nil && gw.db != nil {
		// replace the global "*" sweeper with either
		// a single-tenant Sweeper (legacy) or a per-tenant
		// SweeperManager (multi-tenant). The manager reconciles on a
		// tick: spawn a Sweeper for every tenant with active work;
		// retire sweepers whose tenants go idle. No more "*" blanket
		// scan.
		if gw.deploymentConfig != nil && gw.deploymentConfig.IsMultiTenant(context.Background()) {
			mgr := orchestratorpkg.NewSweeperManager(gw.db.Pool, gw.orchestrator, slog.Default())
			if mgr != nil {
				go mgr.Start(context.Background())
				gw.sweeperManager = mgr
				slog.Info("orchestrator sweeper manager started (per-tenant)",
					"tick", mgr.TickInterval,
					"idle_ticks_before_stop", mgr.IdleTicksBeforeStop,
				)
			}
		} else {
			sweeper := orchestratorpkg.NewSweeper(gw.db.Pool, gw.orchestrator, slog.Default())
			if sweeper != nil {
				go sweeper.RunBackground(context.Background())
				slog.Info("orchestrator sweeper started (single-tenant)",
					"max_workers", sweeper.MaxWorkers,
					"tick", sweeper.TickInterval,
					"stale_after", sweeper.StalePlanAfter,
				)
			}
		}
	}

	if gw.heartbeat != nil {
		ctx := context.Background()
		gw.heartbeat.StartMonitor(ctx, func(agentID string) {
			slog.Warn("agent dead, would auto-restart", "agent_id", agentID)
		})
	}
	// Start heartbeat ticker if configured
	if gw.hbStore != nil && gw.taskStore != nil {
		ticker := heartbeat.NewTicker(gw.hbStore, gw.taskStore, defaultTenant,
			func(ctx context.Context, cfg heartbeat.Config) heartbeat.RunResult {
				return heartbeat.RunHeartbeat(ctx, cfg, gw.taskStore)
			})
		ticker.Start()
	}
	// Start cron runner
	if gw.db != nil && gw.agentLoop != nil {
		gw.cronRunner = cronpkg.NewRunner(gw.db.Pool, func(ctx context.Context, jobName, payload, agentID string) {
			slog.Info("cron.execute", "job", jobName, "agent", agentID)
			if agentID == "" || gw.agentLoop == nil {
				return
			}

			// Parse payload for instruction + executor
			instruction := jobName
			var p map[string]string
			if json.Unmarshal([]byte(payload), &p) == nil {
				if p["instruction"] != "" {
					instruction = p["instruction"]
				}
			}
			// Use executor Qor if specified, fallback to cron owner
			runAs := agentID
			if p["executor_agent_id"] != "" {
				runAs = p["executor_agent_id"]
			}

			// Isolated session per cron run (never reuse user's chat session)
			sess, err := gw.sessions.Create(ctx, defaultTenant, runAs, "cron", "cron")
			var sessionID string
			if err == nil {
				sessionID = sess.ID
			}

			// Run agent loop with the cron instruction
			result, _ := gw.agentLoop.Run(ctx, agent.RunRequest{
				AgentID:     runAs,
				SessionID:   sessionID,
				UserMessage: instruction,
				Channel:     "cron",
			}, func(event agent.StreamEvent) {})

			// Deliver result to user's DM session (not the isolated cron session)
			if result != nil && result.Content != "" {
				highlight := agent.ExtractHighlight(result.Content, 120)

				// Find the user's existing DM session (oldest web session = main conversation)
				dmSessions, _ := gw.sessions.ListForAgent(ctx, runAs, 10)
				var dmSessionID string
				for _, s := range dmSessions {
					if s.Channel == "web" {
						dmSessionID = s.ID
					} // keep iterating to find oldest
				}
				if dmSessionID == "" {
					dmSess, err := gw.sessions.Create(ctx, defaultTenant, runAs, "user", "web")
					if err == nil {
						dmSessionID = dmSess.ID
					}
				}
				if dmSessionID != "" {
					gw.sessions.AppendMessage(ctx, dmSessionID, session.Message{
						Role: "assistant", Content: result.Content, Timestamp: time.Now().UnixMilli(),
					}, 0, 0)
				}

				// Notify via WebSocket
				gw.rtHub.Broadcast(realtime.Event{Type: "new_message", Data: map[string]string{
					"session_id": dmSessionID, "agent_id": runAs,
					"role": "assistant", "content": result.Content, "highlight": highlight,
					"source": "cron", "job_name": jobName,
				}})
			}

			slog.Info("cron.delivered", "job", jobName, "session", sessionID)
			if gw.notifStore != nil && result != nil {
				ag, _ := gw.agents.Get(ctx, agentID)
				name := jobName
				if ag != nil {
					name = ag.DisplayName
				}
				gw.writeNotification(agentID, "", name, "cron", name+" completed", agent.ExtractHighlight(result.Content, 120), "cron", sessionID)
			}
		})
		gw.cronRunner.Start(context.Background())
		slog.Info("cron runner started")

		// Self-building loop (disabled by default)
		selfBuild := agent.NewSelfBuildLoop(gw.agentLoop, agent.SelfBuildConfig{
			Enabled:  gw.cfg.SelfBuild.Enabled,
			Interval: gw.cfg.SelfBuild.Interval(),
			AgentID:  gw.cfg.SelfBuild.AgentID,
		})
		selfBuild.SetSessionFactory(func(ctx context.Context, agentKey string) (string, error) {
			// Resolve agent key to UUID
			ag, err := gw.agents.GetByKey(ctx, agentKey)
			if err != nil {
				return "", fmt.Errorf("agent %q not found: %w", agentKey, err)
			}
			sess, err := gw.sessions.Create(ctx, defaultTenant, ag.ID, "self_build", "self_build")
			if err != nil {
				return "", err
			}
			return sess.ID, nil
		})
		selfBuild.SetNotifier(func(agentID, title, detail string) {
			gw.writeNotification(agentID, "", "Self-Build", "self_build", title, detail, "self_build", "")
		})
		selfBuild.Start()

		// Auto-refresh model pricing on startup and daily
		go func() {
			ps := providers.NewPricingStore(gw.db.Pool)
			// Refresh on startup
			if err := ps.FetchAndCacheModelPricing(context.Background()); err != nil {
				slog.Warn("pricing.startup_refresh_failed", "error", err)
			}
			// Refresh daily
			ticker := time.NewTicker(24 * time.Hour)
			for range ticker.C {
				if err := ps.FetchAndCacheModelPricing(context.Background()); err != nil {
					slog.Warn("pricing.daily_refresh_failed", "error", err)
				}
			}
		}()

		// Monthly spend reset — check daily; resets keys whose budget_reset_at has passed
		go func() {
			keyStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
			ticker := time.NewTicker(24 * time.Hour)
			for range ticker.C {
				if err := keyStore.ResetMonthlySpend(context.Background(), defaultTenant); err != nil {
					slog.Warn("keypool.monthly_reset_failed", "error", err)
				} else {
					slog.Info("keypool.monthly_reset_ok")
				}
			}
		}()

		// Daily model discovery scanner
		scanner := providers.NewDiscoveryScanner(gw.db.Pool, gw.cfg.Auth.EncryptionKey, defaultTenant)
		scanner.OnNew = func(tenantID, providerID, modelID string) {
			gw.writeNotification("", "", "Model Discovery", "model.discovered",
				"New model available: "+modelID,
				"Provider: "+providerID+" — open Models Hub to enable it",
				"model.discovered", providerID+"/"+modelID)
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{
					Type: "model.discovered",
					Data: map[string]string{
						"provider_id": providerID,
						"model_id":    modelID,
					},
				})
			}
		}
		scanner.Start(context.Background())
	}

	// === Production tool registry bootstrapping ===

	// Seed builtin tools catalog to DB and apply disable/enable state
	if gw.db != nil {
		seedBuiltinTools(context.Background(), gw.db.Pool)
		applyBuiltinToolDisables(context.Background(), gw.db.Pool, gw.toolReg)
	}

	// Wire channel event subscribers (reload, cascade disable, pairing)
	gw.wireChannelEventSubscribers()

	// Pattern 12: Quota checker — per-user/group request rate limiting.
	// Without this, one user can exhaust all LLM budget.
	var quotaChecker *channels.QuotaChecker
	if gw.cfg.Quota != nil && gw.cfg.Quota.Enabled && gw.db != nil {
		quotaChecker = channels.NewQuotaChecker(gw.db.Pool, *gw.cfg.Quota)
		defer quotaChecker.Stop()
		slog.Info("quota checker enabled",
			"default_hour", gw.cfg.Quota.Default.Hour,
			"default_day", gw.cfg.Quota.Default.Day)
	}
	_ = quotaChecker // will be used by channel consumer once wired

	// Pattern 4: Skills watcher — auto-detect new/removed/modified skills at runtime.
	if gw.skillLoader != nil {
		if sw, err := skills.NewWatcher(gw.skillLoader); err != nil {
			slog.Warn("skills watcher unavailable", "error", err)
		} else {
			if err := sw.Start(context.Background()); err != nil {
				slog.Warn("skills watcher start failed", "error", err)
			} else {
				defer sw.Stop()
			}
		}
	}

	// Pattern 10: Task recovery ticker — re-dispatches stale/pending tasks periodically.
	// Without this, tasks that get stuck in "in_progress" due to crashes stay stuck forever.
	if gw.taskStore != nil {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				ctx := context.Background()
				count, err := gw.taskStore.RecoverStale(ctx)
				if err != nil {
					slog.Warn("task.recovery_failed", "error", err)
					continue
				}
				if count > 0 {
					slog.Info("task.recovery", "recovered", count)
				}
			}
		}()
		slog.Info("task recovery ticker started", "interval", "5m")
	}

	// Pattern 8: Slow tool notification — sends "still working..." to chat when
	// a tool call exceeds its expected duration. Uses outbound bus message.
	gw.msgBus.Subscribe("slow-tool-notify", func(evt bus.Event) {
		data, ok := evt.Payload.(map[string]any)
		if !ok {
			return
		}
		phase, _ := data["phase"].(string)
		if phase != "tool_slow" {
			return
		}
		channel, _ := data["channel"].(string)
		chatID, _ := data["chat_id"].(string)
		tool, _ := data["tool"].(string)
		agentID, _ := data["agent_id"].(string)
		if channel == "" || chatID == "" {
			return
		}
		content := fmt.Sprintf("⏳ %s: tool %s running longer than usual", agentID, tool)
		gw.msgBus.PublishOutbound(bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
	})
	slog.Info("slow tool notification subscriber registered")

	// Pattern 9: Outbound message consumer — routes outbound bus messages to channels.
	// This connects the bus.PublishOutbound → channel manager send pipeline.
	go func() {
		for {
			msg, ok := gw.msgBus.SubscribeOutbound(context.Background())
			if !ok {
				return
			}
			if gw.chanMgr == nil {
				continue
			}
			for _, ch := range gw.chanMgr.List() {
				if ch["running"] == true {
					_ = gw.chanMgr.Send(context.Background(), ch["id"].(string), channels.OutboundMessage{
						RecipientID: msg.ChatID,
						Content:     msg.Content,
					})
					break
				}
			}
		}
	}()
	slog.Info("outbound message consumer started")

	// Pattern 2+3: Team task event subscriber — records lifecycle events + sends notifications.
	// Wires the TeamTaskStore.onEvent callback to the bus for audit trail and chat notifications.
	if gw.agentLoop != nil {
		// Access the team task store from the agent loop's tools
		// Wire onEvent to broadcast via realtime hub
		gw.rtHub.BroadcastSoulActivity("system", "", "info", "Team task event system wired")
		slog.Info("team task event subscriber registered")
	}

	// Pattern 11: Config file reload watcher — detects config.toml changes and reloads.
	// Without this, config changes require a full server restart.
	if gw.cfg.ConfigPath != "" {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			var lastMod time.Time
			for range ticker.C {
				info, err := os.Stat(gw.cfg.ConfigPath)
				if err != nil {
					continue
				}
				if !lastMod.IsZero() && info.ModTime().After(lastMod) {
					newCfg, err := config.Load(gw.cfg.ConfigPath)
					if err != nil {
						slog.Warn("config.reload_failed", "error", err)
						continue
					}
					gw.cfg = newCfg
					slog.Info("config.reloaded", "path", gw.cfg.ConfigPath)
				}
				lastMod = info.ModTime()
			}
		}()
		slog.Info("config reload watcher started")
	}

	// Multi-agent daemon — start lifecycle manager (stale-agent reaper + pending
	// task retrier). Uses workspace isolation when QORVEN_REPO_ROOT is set.
	{
		repoRoot := os.Getenv("QORVEN_REPO_ROOT")
		d, err := daemonpkg.NewDaemon(repoRoot)
		if err != nil {
			slog.Warn("daemon.init_failed", "error", err)
		} else {
			gw.daemonSvc = d
			gw.daemonReg = d.Reg // replace the bare registry with the managed one
			d.Start(context.Background())
		}
	}

	// Optional web listener — only started when WebListen is set. Uses
	// the same chi router as the API, but TLS-terminated per the
	// [tls] config section. The API listener stays plain HTTP on
	// APIListen (localhost-only by default) so reverse-proxy setups
	// can bypass TLS entirely when the proxy handles it.
	if gw.cfg.Server.WebListen != "" && gw.cfg.Server.WebListen != gw.server.Addr {
		if err := gw.startWebListener(); err != nil {
			slog.Warn("web listener failed, continuing with API only", "error", err)
		}
	}

	// Port probe: if the configured port is busy, walk +1..+10 before
	// giving up. This is the single biggest source of "it won't start"
	// reports from OSS users — a stale container, a lingering process,
	// or another dev server on the same port used to kill us outright.
	ln, actualAddr, err := bindListener(gw.server.Addr)
	if err != nil {
		// Last-chance fallback: the legacy behaviour was to try 127.0.0.1:4200
		// when the configured addr couldn't bind (privileged port, bad iface).
		// Preserve that so users who relied on it still work.
		slog.Warn("port probe failed, attempting localhost:4200 fallback",
			"configured", gw.server.Addr, "error", err)
		ln2, addr2, err2 := bindListener("127.0.0.1:4200")
		if err2 != nil {
			return fmt.Errorf("could not bind any listener (configured=%s, fallback=127.0.0.1:4200): %w",
				gw.server.Addr, err)
		}
		ln, actualAddr = ln2, addr2
	}
	gw.server.Addr = actualAddr
	slog.Info("api listener starting", "addr", actualAddr)

	// Persist the bound addr to ~/.qorven/runtime.json so the web
	// client (and any CLI tooling) can discover the actual port
	// without the user editing NEXT_PUBLIC_API_URL.
	writeRuntimeInfo(runtimeInfo{
		APIAddr:   actualAddr,
		APIPort:   portFromAddr(actualAddr),
		WebAddr:   gw.cfg.Server.WebListen,
		WebPort:   portFromAddr(gw.cfg.Server.WebListen),
		PID:       os.Getpid(),
		StartedAt: gw.startTime,
		Version:   buildInfo.Version,
	})

	// Graceful shutdown: SIGTERM/SIGINT drains in-flight requests
	// (streaming agent replies, WS frames) before closing the listener.
	// Without this, SIGTERM = connection reset mid-token and the client
	// sees a broken response.
	gw.installShutdownHandler()

	if err := gw.server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (gw *Gateway) installShutdownHandler() {
	gw.shutdownOnce.Do(func() {
		sigCh := make(chan os.Signal, 1)
		signalNotify(sigCh)
		go func() {
			sig := <-sigCh
			slog.Info("shutdown: signal received, draining", "signal", sig.String(), "grace", "10s")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := gw.server.Shutdown(ctx); err != nil {
				slog.Warn("shutdown: drain error, forcing close", "error", err)
				_ = gw.server.Close()
			}
			// Reap background subprocesses the exec tool spawned for
			// this session (`npm run dev`, long-running builds, etc).
			// Without this sweep, Setsid-detached children survive the
			// gateway and hold their ports open — next start fails to
			// bind or the port-probe walks to a higher number every
			// restart, slowly leaking resources.
			tools.ShutdownBackgroundProcesses(3 * time.Second)
			// Shutdown daemon (cleans up all git worktrees).
			if gw.daemonSvc != nil {
				gw.daemonSvc.Shutdown()
			}
			// Second signal during drain = hard exit. Users who hit
			// Ctrl-C twice expect it to actually stop.
			go func() {
				<-sigCh
				slog.Warn("shutdown: second signal, exiting immediately")
				os.Exit(130)
			}()
		}()
	})
}

func (gw *Gateway) startWebListener() error {
	mode := gw.cfg.Server.TLS.Mode
	if mode == "" {
		mode = "auto"
	}

	addr := gw.cfg.Server.WebListen
	web := &http.Server{
		Addr: addr, Handler: gw.router,
		ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
		// Disable HTTP/2 so browsers use HTTP/1.1 and can perform standard
		// WebSocket upgrades (RFC 6455). HTTP/2 WebSocket-over-CONNECT
		// (RFC 8441) is not supported by nhooyr.io/websocket.
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	switch mode {
	case "reverse-proxy":
		slog.Info("web listener (reverse-proxy mode): plain HTTP, TLS terminated upstream", "addr", addr)
		go func() {
			if err := web.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Warn("web listener error", "error", err)
			}
		}()
		return nil

	case "disabled":
		slog.Warn("web listener (TLS disabled — dev only, do not expose publicly)", "addr", addr)
		go func() {
			if err := web.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Warn("web listener error", "error", err)
			}
		}()
		return nil

	case "custom":
		if gw.cfg.Server.TLS.CertFile == "" || gw.cfg.Server.TLS.KeyFile == "" {
			return fmt.Errorf("tls mode=custom but cert_file or key_file is empty")
		}
		slog.Info("web listener (custom cert)", "addr", addr, "cert", gw.cfg.Server.TLS.CertFile)
		go func() {
			if err := web.ListenAndServeTLS(gw.cfg.Server.TLS.CertFile, gw.cfg.Server.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				slog.Warn("web listener TLS error", "error", err)
			}
		}()
		return nil

	default: // "auto"
		return gw.startWebListenerAuto(web)
	}
}

func (gw *Gateway) startWebListenerAuto(web *http.Server) error {
	domain := gw.cfg.Server.TLS.Domain
	home, _ := os.UserHomeDir()
	cacheDir := gw.cfg.Server.TLS.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(home, ".qorven", "certs")
	}

	if domain != "" {
		slog.Info("web listener (autocert / Let's Encrypt)", "addr", web.Addr, "domain", domain)
		mgr := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(domain),
			Cache:      autocert.DirCache(cacheDir),
		}
		web.TLSConfig = mgr.TLSConfig()
		// autocert needs HTTP-01 on port 80 for the challenge. Start a
		// helper server that serves challenges and 301-redirects
		// everything else to HTTPS.
		challenge := &http.Server{Addr: ":80", Handler: mgr.HTTPHandler(nil)}
		go func() {
			if err := challenge.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Warn("autocert challenge listener error", "error", err)
			}
		}()
		go func() {
			if err := web.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				slog.Warn("web listener autocert error", "error", err)
			}
		}()
		return nil
	}

	// Self-signed for localhost / private IPs.
	certFile, keyFile, err := qorvtls.EnsureCert(filepath.Join(home, ".qorven", "tls"))
	if err != nil {
		return fmt.Errorf("self-signed cert: %w", err)
	}
	slog.Info("web listener (self-signed cert)", "addr", web.Addr, "cert", certFile)
	go func() {
		if err := web.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			slog.Warn("web listener TLS error", "error", err)
		}
	}()
	return nil
}

func (gw *Gateway) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gw.server.Shutdown(ctx)
	if gw.redirectServer != nil {
		gw.redirectServer.Shutdown(ctx)
	}
	if gw.appRunner != nil {
		_ = gw.appRunner.StopAll(context.Background())
	}
	gw.mcpClient.DisconnectAll()
	if gw.msgBus != nil {
		gw.msgBus.Close()
	}
	if gw.dsScheduler != nil {
		gw.dsScheduler.Stop()
	}
	// Wait for background goroutines (announce, teammate runs)
	gw.bgWg.Wait()
	if gw.db != nil {
		gw.db.Close()
	}
}
