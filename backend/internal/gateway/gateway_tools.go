// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	socialqor "github.com/qorvenai/qorven/internal/qor/social"
	"github.com/qorvenai/qorven/internal/agent"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/dashboard"
	gatewayllm "github.com/qorvenai/qorven/internal/gateway/llm"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/qor/browser"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/sandbox"
	"github.com/qorvenai/qorven/internal/scraper"
	"github.com/qorvenai/qorven/internal/search"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/storage"
	supervisorpkg "github.com/qorvenai/qorven/internal/supervisor"
	"github.com/qorvenai/qorven/internal/tools"
)

func (gw *Gateway) registerTools() {
	workspace := "/tmp/qorven-workspace" // default workspace
	reg := gw.toolReg

	// Filesystem — allow workspace + data dir + /tmp for full coding capability
	dataDir := config.DataDir()
	readTool := tools.NewReadFileTool(workspace)
	readTool.AllowPaths(dataDir, "/tmp")
	reg.Register(readTool)
	writeTool := tools.NewWriteFileTool(workspace)
	writeTool.AllowPaths(dataDir, "/tmp")
	reg.Register(writeTool)
	listTool := tools.NewListFilesTool(workspace)
	listTool.AllowPaths(dataDir, "/tmp")
	reg.Register(listTool)
	editTool := tools.NewEditTool(workspace)
	editTool.AllowPaths(dataDir, "/tmp")
	reg.Register(editTool)

	// Runtime
	reg.Register(tools.NewExecTool(workspace, true))
	// System operations — structured tool for privileged agent roles (sysops).
	// Only executes when the tool context has AllowElevated set (see loop.go).
	reg.Register(tools.NewSystemOpsTool())

	// Web
	// Web search — try to find Perplexity API key from providers
	// Build search pipeline with all available keys
	searchCfg := search.Config{SearXNGURL: ""}
	// Env var fallback for search keys
	if v := os.Getenv("TAVILY_API_KEY"); v != "" {
		searchCfg.TavilyKey = v
	}
	if v := os.Getenv("PERPLEXITY_API_KEY"); v != "" {
		searchCfg.PerplexityKey = v
	}
	if gw.db != nil {
		keyStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
		for _, provID := range []string{"perplexity", "brave", "tavily", "serper", "qor_crawl"} {
			keys, _ := keyStore.ListKeys(context.Background(), defaultTenant, provID)
			for _, k := range keys {
				if k.Status == "verified" {
					if dk, err := providers.DecryptKeyBytes(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey); err == nil {
						switch provID {
						case "perplexity":
							searchCfg.PerplexityKey = string(dk)
						case "brave":
							searchCfg.BraveKey = string(dk)
						case "tavily":
							searchCfg.TavilyKey = string(dk)
						case "serper":
							searchCfg.SerperKey = string(dk)
						case "qor_crawl":
							searchCfg.QorCrawlKey = string(dk)
						}
						break
					}
				}
			}
		}
	}
	searchPipeline := search.NewPipeline(searchCfg, gw.providerReg.Default())
	slog.Info("search.pipeline", "perplexity", searchCfg.PerplexityKey != "", "brave", searchCfg.BraveKey != "", "tavily", searchCfg.TavilyKey != "", "searxng", searchCfg.SearXNGURL != "")
	browserMgr := browser.New(browser.DefaultConfig())
	reg.Register(tools.NewWebSearchTool(searchPipeline))
	webFetchTool := tools.NewWebFetchToolWithConfig(tools.WebFetchConfig{})
	// Full engine router (from qor_crawl) — routes URLs to best scraping engine
	engineRouter := scraper.NewEngineRouter()
	webFetchTool.SetEngineRouter(engineRouter)
	// Wire headless browser fallback for 403/bot-protected sites
	webFetchTool.SetBrowserFallback(func(ctx context.Context, url string) (string, error) {
		if err := browserMgr.Start(ctx); err != nil {
			return "", fmt.Errorf("browser start: %w", err)
		}
		if err := browserMgr.Navigate(ctx, url); err != nil {
			return "", fmt.Errorf("browser navigate: %w", err)
		}
		browserMgr.WaitIdle(ctx, 3e9) // 3 seconds for JS to render
		snap, err := browserMgr.TakeSnapshot(ctx)
		if err != nil {
			return "", fmt.Errorf("browser snapshot: %w", err)
		}
		slog.Info("web_fetch.browser_success", "url", url, "nodes", snap.Stats.Nodes)
		return snap.Tree, nil
	})
	reg.Register(webFetchTool)
	reg.Register(tools.NewClarifyTool())

	// Intake tools — Prime-role exclusive (enforced via ApplyRole, not here).
	// ask_followup_question: surfaces a question to the user during onboarding.
	// produce_project_brief: persists the structured brief to project_briefs.
	reg.Register(NewAskFollowupTool())
	if gw.db != nil {
		reg.Register(NewProduceProjectBriefTool(gw.db.Pool, defaultTenant))
	}

	// Weather — Open-Meteo, no API key needed. Safe to register
	// unconditionally; the tool handles its own HTTP errors if the
	// host is offline.
	reg.Register(tools.NewWeatherTool())

	// Codebase digest — "pack this repo into one LLM-ready blob".
	// Inherits the same allow-list as read_file so both tools see
	// the same paths.
	digestTool := tools.NewCodebaseDigestTool(workspace)
	digestTool.AllowPaths(dataDir, "/tmp")
	reg.Register(digestTool)

	// NL→SQL — user connects a database via Settings → Connections,
	// the registry is populated on boot and refreshed on save. Tools
	// surface whether the registry is empty or has entries.
	gw.sqlRegistry = tools.NewSQLConnectionRegistry()
	loadSQLConnections(context.Background(), gw.db, defaultTenant, gw.cfg.Auth.EncryptionKey, gw.sqlRegistry)
	reg.Register(tools.NewSQLConnectionsTool(gw.sqlRegistry))
	reg.Register(tools.NewSQLSchemaTool(gw.sqlRegistry))
	reg.Register(tools.NewSQLQueryTool(gw.sqlRegistry))

	// Harness-style browser primitives — coordinate-based click/type,
	// vision-first screenshotting, page_info, JS eval. Reuses the
	// existing browserMgr so legacy `browser`/`browse_and_act` tools
	// and these primitives share one Chromium process + profile.
	reg.Register(browser.NewBrowserGotoTool(browserMgr))
	reg.Register(browser.NewBrowserInfoTool(browserMgr))
	reg.Register(browser.NewBrowserScreenshotTool(browserMgr))
	reg.Register(browser.NewBrowserClickTool(browserMgr))
	reg.Register(browser.NewBrowserTypeTool(browserMgr))
	reg.Register(browser.NewBrowserPressTool(browserMgr))
	reg.Register(browser.NewBrowserScrollTool(browserMgr))
	reg.Register(browser.NewBrowserJSTool(browserMgr))
	// computer_use — one-call do-and-see ergonomics layered on top of
	// the primitives. Convenient for iterative UI work; the primitives
	// remain available for one-shot actions that don't need a
	// follow-up screenshot.
	reg.Register(browser.NewComputerUseTool(browserMgr))

	// User-to-agent screen share tool. The user starts/stops sharing
	// from the web UI's "Share Screen" control; this tool just reads
	// whatever the latest frame is. Returns "not sharing" when idle.
	reg.Register(NewUserScreenCaptureTool(gw.screenShare, defaultTenant))

	// Agent-to-user live stream. When enabled via POST /v1/browser/live/start,
	// the browser manager begins emitting JPEG frames on the realtime
	// hub so the web UI can render a live preview. Off by default so
	// headless runs without a viewer don't incur capture cost.
	gw.wireBrowserLivePublisher(browserMgr)

	// Storage (rclone) — 70+ cloud backends behind one binary. Tools
	// register whether rclone is installed or not; when missing they
	// return an actionable "install rclone" message instead of
	// disappearing from the agent's tool list. AllowWrite default is
	// false — admin opts in via Settings → Storage.
	storageMgr := storage.NewManager(storage.Config{
		AllowWrite: readStorageAllowWrite(context.Background(), gw.db, defaultTenant),
	})
	reg.Register(tools.NewStorageRemotesTool(storageMgr))
	reg.Register(tools.NewStorageListTool(storageMgr))
	reg.Register(tools.NewStorageReadTool(storageMgr))
	reg.Register(tools.NewStorageWriteTool(storageMgr))
	reg.Register(tools.NewStorageCopyTool(storageMgr))
	reg.Register(tools.NewStorageSyncTool(storageMgr))
	if storageMgr.Installed() {
		slog.Info("storage.rclone", "available", true, "write_enabled", storageMgr.AllowWrite())
	} else {
		slog.Info("storage.rclone", "available", false, "hint", "install rclone to enable storage_* tools")
	}

	// QorCrawl — deep web crawling
	if fcToken := os.Getenv("CRAWL4AI_API_TOKEN"); fcToken != "" {
		reg.Register(tools.NewQorCrawlTool(fcToken))
		slog.Info("qor_crawl.configured")
	}

	// send_file — agent delivers workspace files to the user as downloads.
	// The onSend callback emits a notification so the user sees a download link.
	reg.Register(tools.NewSendFileTool(workspace, func(token, filename, mime string) {
		gw.writeNotification("", "", "", "file_download",
			"File ready: "+filename,
			"/api/v1/files/download/"+token,
			"file", token)
	}))

	// Memory + KG (need DB)
	if gw.db != nil {
		reg.Register(tools.NewMemorySearchTool(gw.db.Pool))
		reg.Register(tools.NewMemoryGetTool(gw.db.Pool))
		reg.Register(tools.NewKGSearchTool(gw.db.Pool))
	}

	// Connector JIT loader — lets agents fetch action catalogues on demand
	// instead of having the full catalogue inlined in the system prompt every turn.
	if gw.connKB != nil {
		reg.Register(newListConnectorActionsTool(gw.connKB, defaultTenant))
	}

	// MCP JIT loader — symmetric to connector JIT, for MCP server tool catalogues.
	if gw.mcpManager != nil {
		reg.Register(newListMCPToolsTool(gw.mcpManager, defaultTenant, ""))
	}

	// Sessions (need DB)
	if gw.db != nil {
		reg.Register(tools.NewSessionsListTool(gw.db.Pool))
		reg.Register(tools.NewSessionsHistoryTool(gw.db.Pool))
		reg.Register(tools.NewSessionStatusTool(gw.db.Pool))
		// cron is NOT wrapped with a permission gate: it is a core autonomous capability.
		// Every role that should have scheduling access sets cron=auto_approved in
		// roleDefaults (defaults.go). A blocking per-call gate causes context deadline
		// timeouts for channel sessions (Telegram etc.) where no human is present to
		// approve, trips the circuit breaker, and breaks the feature entirely.
		reg.Register(tools.NewCronTool(gw.db.Pool))
		dmTool := tools.NewSendDMTool(gw.db.Pool, &chanAdapter{gw: gw})
		reg.Register(dmTool)
		reg.Register(tools.NewSendTelegramTool(dmTool))
	}

	// Task management — create_task lets an agent spawn background work linked to the current discussion.
	if gw.taskStore != nil {
		reg.Register(NewCreateTaskTool(gw))
	}

	// Automation
	reg.Register(tools.NewDateTimeTool())

	// LSP (semantic code navigation)
	reg.Register(tools.NewLSPTool())

	// Email — wire SMTP/IMAP from config or env vars
	emailSend := tools.NewEmailSendTool()
	emailRead := tools.NewEmailReadTool()
	smtpHost := gw.cfg.Email.SMTPHost
	if smtpHost == "" {
		smtpHost = os.Getenv("SMTP_HOST")
	}
	if smtpHost != "" {
		smtpUser := gw.cfg.Email.SMTPUser
		if smtpUser == "" {
			smtpUser = os.Getenv("SMTP_USER")
		}
		smtpPass := gw.cfg.Email.SMTPPass
		if smtpPass == "" {
			smtpPass = os.Getenv("SMTP_PASS")
		}
		smtpFrom := gw.cfg.Email.From
		if smtpFrom == "" {
			smtpFrom = os.Getenv("SMTP_FROM")
		}
		smtpPort := gw.cfg.Email.SMTPPort
		if smtpPort == 0 {
			smtpPort = 465
		}
		mailCfg := &tools.MailboxConfig{
			SMTP: &tools.SMTPConfig{
				Host: smtpHost, Port: smtpPort,
				User: smtpUser, Password: smtpPass,
				From: smtpFrom, FromName: gw.cfg.Email.FromName,
			},
			IMAP: &tools.IMAPConfig{
				Host: os.Getenv("IMAP_HOST"), Port: 993,
				User: os.Getenv("IMAP_USER"), Password: os.Getenv("IMAP_PASS"),
			},
			Pool: gw.db.Pool,
		}
		emailSend.SetMailbox(mailCfg)
		emailRead.SetMailbox(mailCfg)
		slog.Info("email.configured", "smtp", smtpHost, "from", smtpFrom)

		// Wire outbound approval notifications to WebSocket
		tools.OnApprovalQueued = func(queueID, agentID, toolName string, args map[string]any) {
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{
					Type: "approval_required",
					Data: map[string]string{"queue_id": queueID, "agent_id": agentID, "tool": toolName},
				})
			}
			// Send Telegram notification to owner
			go func() {
				detail := ""
				if to, ok := args["to"].(string); ok {
					detail = " to " + to
				}
				if subj, ok := args["subject"].(string); ok {
					detail += " (" + subj + ")"
				}
				msg := fmt.Sprintf("🔔 Approval needed\nAgent wants to: %s%s\n\nReply:\n/approve %s\n/deny %s", toolName, detail, queueID[:8], queueID[:8])
				gw.sendToChannel(context.Background(), agentID, "", msg, "", "")
				// Create persistent notification
				gw.writeNotification(agentID, "", "", "approval", "Approval needed: "+toolName+detail, "", "approval", queueID)
			}()
			// Send email notification to owner
			go func() {
				if gw.cfg.Auth.OwnerEmail != "" {
					detail := ""
					if to, ok := args["to"].(string); ok {
						detail = " to " + to
					}
					subject := fmt.Sprintf("Qorven: Approval needed — %s", toolName)
					body := fmt.Sprintf("Agent wants to: %s%s\n\nApprove: %s/approvals?approve=%s\nDeny: %s/approvals?deny=%s",
						toolName, detail, gw.cfg.Server.BaseURL, queueID, gw.cfg.Server.BaseURL, queueID)
					gw.sendToChannel(context.Background(), agentID, gw.cfg.Auth.OwnerEmail, body, subject, "")
				}
			}()
		}
	}

	// Wire file → drive sync + project hooks
	if gw.driveStore != nil {
		tools.OnFileWritten = func(ctx context.Context, agentID, name, path string, size int64) {
			mime := agent.MimeFromExt(filepath.Ext(path))
			gw.driveStore.CreateFile(ctx, defaultTenant, agentID, filepath.Base(name), path, mime, size, false, nil)
			// Track file in session
			if sid := tools.SessionIDFromCtx(ctx); sid != "" && gw.sessions != nil {
				gw.sessions.TrackFile(ctx, sid, path, "modified")
			}
			// Fire project hooks for file change
			if gw.projectHooks != nil && gw.projectReg != nil {
				ws := tools.WorkspaceFromCtx(ctx)
				if ws != "" {
					gw.projectHooks.FireFileEvent(ctx, ws, agent.PHookFileChanged, path)
				}
			}
		}
	}

	// Wire cron → calendar + in-memory scheduler
	if gw.db != nil {
		tools.OnCronCreated = func(ctx context.Context, agentID, jobID, name, expression, task string) {
			gw.db.Pool.Exec(ctx, `INSERT INTO calendar_events (tenant_id, agent_id, title, description, event_type, source_id, recurrence) VALUES ($1, $2, $3, $4, 'cron', $5, $6)`,
				defaultTenant, agentID, name, task, jobID, expression)
		}
	}
	// Wire cron → DB: set next_run_at so the DB-backed cronRunner picks it up.
	// gw.brain.Cron is NOT used for user-scheduled jobs — only gw.cronRunner polls the DB.
	tools.OnCronSchedule = func(ctx context.Context, tenantID, agentID, jobID, name, expression, task string) {
		if gw.db == nil {
			return
		}
		nextRun := cronpkg.NextRunFromExpr(expression)
		if _, err := gw.db.Pool.Exec(ctx,
			`UPDATE cron_jobs SET next_run_at = $1 WHERE id = $2`,
			nextRun, jobID,
		); err != nil {
			slog.Warn("cron.schedule_set_next_failed", "job", name, "err", err)
		}
	}
	tools.OnCronRemove = func(jobID string) {
		// DB-backed runner: disabling the row is enough; no in-memory state to clean.
		if gw.db == nil {
			return
		}
		gw.db.Pool.Exec(context.Background(), `UPDATE cron_jobs SET enabled = false WHERE id = $1`, jobID)
	}

	// Wire exec → sandbox_runs
	if gw.db != nil {
		tools.OnExecComplete = func(ctx context.Context, agentID, command, output string, exitCode int, durationMs int64) {
			gw.db.Pool.Exec(ctx, `INSERT INTO sandbox_runs (agent_id, command, output, exit_code, duration_ms, status) VALUES ($1, $2, $3, $4, $5, 'completed')`,
				agentID, command, output[:min(len(output), 10000)], exitCode, durationMs)
		}

		// Wire manage_agents callbacks
		tools.OnAgentCreate = func(ctx context.Context, name, model, role, prompt string) (string, error) {
			a, err := gw.agents.Create(ctx, defaultTenant, agent.CreateAgentInput{
				AgentKey: strings.ToLower(strings.ReplaceAll(name, " ", "-")), DisplayName: name,
				Model: model, Role: role, SystemPrompt: prompt, Temperature: 0.5,
				ContextWindow: 128000, MaxToolIterations: 20, ToolProfile: "full",
			})
			if err != nil {
				return "", err
			}
			// Seed archetype bundles so the agent has soul/identity/tools from day one.
			if gw.bundleStore != nil {
				soulContent := prompt
				if soulContent == "" {
					if seed, ok := agent.AgentSeeds[role]; ok && seed.Soul != "" {
						soulContent = seed.Soul
					} else {
						soulContent = fmt.Sprintf("You are %s, an AI specialist.", name)
					}
				}
				gw.bundleStore.Upsert(ctx, agent.Bundle{
					AgentID: a.ID, BundleType: "soul", Name: "soul",
					Content: soulContent, Priority: 200, Enabled: true,
				})
				gw.bundleStore.SeedDefaults(ctx, a.ID, role)
			}
			// Activate runtime so the agent can receive delegated tasks immediately.
			if gw.runtimeMgr != nil {
				gw.runtimeMgr.EnsureRuntime(a.ID, defaultTenant)
			}
			return a.ID, nil
		}
		tools.OnAgentList = func(ctx context.Context) ([]map[string]string, error) {
			list, err := gw.agents.List(ctx, defaultTenant)
			if err != nil {
				return nil, err
			}
			out := []map[string]string{}
			for _, a := range list {
				out = append(out, map[string]string{"id": a.ID, "name": a.DisplayName, "model": a.Model, "role": func() string {
					if a.Role != nil {
						return *a.Role
					}
					return ""
				}()})
			}
			return out, nil
		}
		tools.OnAgentUpdate = func(ctx context.Context, id string, fields map[string]any) error {
			return gw.agents.Update(ctx, id, fields)
		}
		tools.OnAgentDelete = func(ctx context.Context, id string) error {
			return gw.agents.Delete(ctx, id)
		}

		// Wire hr_manage callbacks — CHRO/COO org lifecycle tool
		tools.OnHRHireAgent = func(ctx context.Context, name, model, role, orgRole, orgLevel, dept, prompt string, monthlyBudgetUSD float64, managerAgentID string) (string, error) {
			a, err := gw.agents.Create(ctx, defaultTenant, agent.CreateAgentInput{
				AgentKey:       strings.ToLower(strings.ReplaceAll(name, " ", "-")),
				DisplayName:    name,
				Model:          model,
				Role:           role,
				OrgRole:        orgRole,
				OrgLevel:       orgLevel,
				ManagerID:      managerAgentID,
				SystemPrompt:   prompt,
				Temperature:    0.5,
				ContextWindow:  128000,
				MaxToolIterations: 20,
				ToolProfile:    "full",
			})
			if err != nil {
				return "", err
			}
			// Set monthly budget
			if monthlyBudgetUSD > 0 {
				_, _ = gw.db.Pool.Exec(ctx,
					`UPDATE agents SET monthly_budget_usd=$1 WHERE id=$2`,
					monthlyBudgetUSD, a.ID)
			}
			// L1/L2 agents can delegate tasks down the hierarchy
			if orgLevel == "l1" || orgLevel == "l2" {
				_, _ = gw.db.Pool.Exec(ctx,
					`UPDATE agents SET can_delegate=true WHERE id=$1`, a.ID)
			}
			// Seed archetype soul + defaults
			if gw.bundleStore != nil {
				soulContent := prompt
				if soulContent == "" {
					if seed, ok := agent.AgentSeeds[orgRole]; ok && seed.Soul != "" {
						soulContent = seed.Soul
					} else if seed, ok := agent.AgentSeeds[role]; ok && seed.Soul != "" {
						soulContent = seed.Soul
					} else {
						soulContent = fmt.Sprintf("You are %s, an AI specialist in the %s department.", name, dept)
					}
				}
				gw.bundleStore.Upsert(ctx, agent.Bundle{
					AgentID: a.ID, BundleType: "soul", Name: "soul",
					Content: soulContent, Priority: 200, Enabled: true,
				})
				gw.bundleStore.SeedDefaults(ctx, a.ID, role)
			}
			// Write to org_roster
			gw.db.Pool.Exec(ctx,
				`INSERT INTO org_roster (tenant_id, agent_id, org_level, org_role, display_name, status, hired_by)
				 VALUES ($1,$2,$3,$4,$5,'active',$6) ON CONFLICT DO NOTHING`,
				defaultTenant, a.ID, orgLevel, orgRole, name, managerAgentID)
			// Activate runtime
			if gw.runtimeMgr != nil {
				gw.runtimeMgr.EnsureRuntime(a.ID, defaultTenant)
			}
			return a.ID, nil
		}

		tools.OnHRTerminateAgent = func(ctx context.Context, agentID, reason, terminatedBy string) error {
			_, err := gw.db.Pool.Exec(ctx,
				`UPDATE agents SET terminated_at=now(), status='suspended' WHERE id=$1 AND tenant_id=$2`,
				agentID, defaultTenant)
			if err != nil {
				return err
			}
			// Snapshot spend into org_roster
			gw.db.Pool.Exec(ctx,
				`UPDATE org_roster SET
				    status='terminated', terminated_at=now(), terminated_by=$1, termination_reason=$2,
				    total_spend_usd=(SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend WHERE agent_id=$3),
				    total_tokens_in=(SELECT COALESCE(SUM(tokens_in),0) FROM org_daily_spend WHERE agent_id=$3),
				    total_tokens_out=(SELECT COALESCE(SUM(tokens_out),0) FROM org_daily_spend WHERE agent_id=$3)
				 WHERE agent_id=$3 AND status='active'`,
				terminatedBy, reason, agentID)
			return nil
		}

		tools.OnHRListOrg = func(ctx context.Context) ([]map[string]any, error) {
			rows, err := gw.db.Pool.Query(ctx,
				`SELECT id, display_name, COALESCE(org_level,'l3'), COALESCE(org_role,''), status,
				        COALESCE(monthly_budget_usd,0), hired_at
				 FROM agents WHERE tenant_id=$1 AND deleted_at IS NULL
				 ORDER BY CASE COALESCE(org_level,'l3') WHEN 'l1' THEN 0 WHEN 'l2' THEN 1 ELSE 2 END, display_name`,
				defaultTenant)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var out []map[string]any
			for rows.Next() {
				var id, name, orgLevel, orgRole, status string
				var budget float64
				var hiredAt *string
				rows.Scan(&id, &name, &orgLevel, &orgRole, &status, &budget, &hiredAt)
				entry := map[string]any{"id": id, "name": name, "org_level": orgLevel, "org_role": orgRole, "status": status, "monthly_budget_usd": budget}
				if hiredAt != nil {
					entry["hired_at"] = *hiredAt
				}
				out = append(out, entry)
			}
			return out, nil
		}

		tools.OnHRGetAgent = func(ctx context.Context, idOrName string) (map[string]any, error) {
			var a agent.Agent
			var err error
			if len(idOrName) == 36 { // UUID
				ap, e := gw.agents.Get(ctx, idOrName)
				if e == nil {
					a = *ap
				}
				err = e
			} else {
				ap, e := gw.agents.GetByKey(ctx, strings.ToLower(strings.ReplaceAll(idOrName, " ", "-")))
				if e == nil {
					a = *ap
				}
				err = e
			}
			if err != nil {
				return nil, fmt.Errorf("agent not found: %s", idOrName)
			}
			role := ""
			if a.Role != nil {
				role = *a.Role
			}
			return map[string]any{
				"id": a.ID, "name": a.DisplayName, "role": role,
				"org_level": a.OrgLevel, "org_role": a.OrgRole,
				"status": a.Status, "model": a.Model,
			}, nil
		}

		// Wire fleet_status callback — live fleet health for executive agents
		tools.OnFleetStatus = func(ctx context.Context) (tools.FleetStatusData, error) {
			pool := gw.db.Pool
			data := tools.FleetStatusData{TierBreakdown: map[string]int{}}

			// Agent counts by status and tier
			rows, err := pool.Query(ctx,
				`SELECT id, display_name, COALESCE(org_role,''), COALESCE(org_level,'l3'), status, updated_at
				 FROM agents WHERE tenant_id=$1 AND deleted_at IS NULL`, defaultTenant)
			if err != nil {
				return data, err
			}
			defer rows.Close()
			for rows.Next() {
				var id, name, orgRole, orgLevel, status string
				var updatedAt time.Time
				rows.Scan(&id, &name, &orgRole, &orgLevel, &status, &updatedAt)
				data.TotalAgents++
				data.TierBreakdown[orgLevel]++
				switch status {
				case "active":
					data.ActiveAgents++
				case "error":
					data.ErrorAgents++
					data.RecentErrors = append(data.RecentErrors, tools.AgentError{
						AgentName: name, Error: "agent in error state", At: updatedAt.Format(time.RFC3339),
					})
				default:
					data.IdleAgents++
				}
				data.Agents = append(data.Agents, tools.AgentSummary{
					ID: id, DisplayName: name, OrgRole: orgRole, OrgLevel: orgLevel,
					Status: status, LastActive: updatedAt.Format(time.RFC3339),
				})
			}

			// Today's spend per agent
			spendRows, err := pool.Query(ctx,
				`SELECT agent_id, cost_usd FROM org_daily_spend WHERE tenant_id=$1 AND date=CURRENT_DATE`, defaultTenant)
			if err == nil {
				defer spendRows.Close()
				spendMap := map[string]float64{}
				for spendRows.Next() {
					var aid string
					var cost float64
					spendRows.Scan(&aid, &cost)
					spendMap[aid] = cost
				}
				for i := range data.Agents {
					data.Agents[i].SpendToday = spendMap[data.Agents[i].ID]
				}
			}

			// Sessions today
			var sessCount int
			pool.QueryRow(ctx,
				`SELECT COUNT(*) FROM sessions WHERE tenant_id=$1 AND created_at >= CURRENT_DATE`, defaultTenant).Scan(&sessCount)
			data.SessionsToday = sessCount

			return data, nil
		}

		// Wire org_finance callback — cost/budget data for CFO/CAIO agents
		tools.OnOrgFinance = func(ctx context.Context, days int) (tools.OrgFinanceData, error) {
			pool := gw.db.Pool
			data := tools.OrgFinanceData{}

			// Month-to-date total
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date >= date_trunc('month', CURRENT_DATE)`, defaultTenant).Scan(&data.TotalMonthUSD)

			// Today's total
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date = CURRENT_DATE`, defaultTenant).Scan(&data.TotalTodayUSD)

			// Yesterday for day-over-day
			var yesterday float64
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date = CURRENT_DATE - 1`, defaultTenant).Scan(&yesterday)
			if yesterday > 0 {
				pct := ((data.TotalTodayUSD - yesterday) / yesterday) * 100
				if pct >= 0 {
					data.DayOverDay = fmt.Sprintf("+%.0f%%", pct)
				} else {
					data.DayOverDay = fmt.Sprintf("%.0f%%", pct)
				}
			} else {
				data.DayOverDay = "n/a"
			}

			// Top spenders this month
			spenderRows, err := pool.Query(ctx,
				`SELECT a.display_name, COALESCE(a.org_role,''), SUM(s.cost_usd), SUM(s.tokens_in), SUM(s.tokens_out)
				 FROM org_daily_spend s JOIN agents a ON a.id = s.agent_id
				 WHERE s.tenant_id=$1 AND s.date >= date_trunc('month', CURRENT_DATE)
				 GROUP BY a.display_name, a.org_role ORDER BY SUM(s.cost_usd) DESC LIMIT 10`, defaultTenant)
			if err == nil {
				defer spenderRows.Close()
				for spenderRows.Next() {
					var sp tools.SpenderAgent
					spenderRows.Scan(&sp.DisplayName, &sp.OrgRole, &sp.MonthCostUSD, &sp.TokensIn, &sp.TokensOut)
					data.TopSpenders = append(data.TopSpenders, sp)
				}
			}

			// Budget utilization
			budgetRows, err := pool.Query(ctx,
				`SELECT a.display_name, COALESCE(a.org_role,''), a.monthly_budget_usd,
				        COALESCE((SELECT SUM(cost_usd) FROM org_daily_spend WHERE agent_id=a.id AND date >= date_trunc('month', CURRENT_DATE)),0)
				 FROM agents a
				 WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND a.monthly_budget_usd > 0
				 ORDER BY a.monthly_budget_usd DESC`, defaultTenant)
			if err == nil {
				defer budgetRows.Close()
				for budgetRows.Next() {
					var b tools.BudgetAgent
					budgetRows.Scan(&b.DisplayName, &b.OrgRole, &b.MonthlyBudget, &b.SpentUSD)
					if b.MonthlyBudget > 0 {
						b.PercentUsed = (b.SpentUSD / b.MonthlyBudget) * 100
					}
					data.BudgetUsage = append(data.BudgetUsage, b)
				}
			}

			// Daily trend
			trendRows, err := pool.Query(ctx,
				`SELECT date::text, SUM(cost_usd) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date >= CURRENT_DATE - $2
				 GROUP BY date ORDER BY date`, defaultTenant, days)
			if err == nil {
				defer trendRows.Close()
				for trendRows.Next() {
					var d tools.DailySpend
					trendRows.Scan(&d.Date, &d.CostUSD)
					data.DailyTrend = append(data.DailyTrend, d)
				}
			}

			return data, nil
		}

		// ── CFO Accounting Tools — bank-grade cost reconciliation, forecasting, budget management ──

		// Reconcile: verify our µUSD math matches provider pricing tables 100%
		tools.OnReconcile = func(ctx context.Context) (tools.ReconciliationReport, error) {
			pool := gw.db.Pool
			report := tools.ReconciliationReport{RunAt: time.Now().UTC().Format(time.RFC3339)}

			// Query all non-cache-hit spend grouped by model
			rows, err := pool.Query(ctx, `
				SELECT model_id, COUNT(*), SUM(tokens_in), SUM(tokens_out),
				       SUM(tokens_thinking), SUM(tokens_cache_write), SUM(tokens_cache_read),
				       SUM(cost_total_uusd)
				FROM gateway_spend_raw
				WHERE tenant_id=$1 AND NOT cache_hit
				GROUP BY model_id
				ORDER BY SUM(cost_total_uusd) DESC`, defaultTenant)
			if err != nil {
				return report, err
			}
			defer rows.Close()

			pricing := gatewayllm.GetPricingSnapshot()
			var totalOurUUSD, totalExpectedUUSD int64
			var totalCalls int
			var modelsWithPricing, modelsTotal int

			for rows.Next() {
				var modelID string
				var calls int
				var tokIn, tokOut, tokThink, tokCacheW, tokCacheR, costUUSD int64
				rows.Scan(&modelID, &calls, &tokIn, &tokOut, &tokThink, &tokCacheW, &tokCacheR, &costUUSD)

				modelsTotal++
				totalCalls += calls
				totalOurUUSD += costUUSD

				mc := tools.ModelReconciliation{
					Model:       modelID,
					TotalCalls:  calls,
					TokensIn:    tokIn,
					TokensOut:   tokOut,
					OurCostUUSD: costUUSD,
				}

				p, ok := pricing[modelID]
				if !ok {
					report.MissingModels = append(report.MissingModels, modelID)
					mc.Match = true // can't check — treat as non-drift
					report.PerModelCheck = append(report.PerModelCheck, mc)
					continue
				}

				modelsWithPricing++
				mc.InputRate = p.InputPer1M
				mc.OutputRate = p.OutputPer1M

				// Recompute expected cost using same toUUSD formula
				expectedIn := int64(math.Round(float64(tokIn) * p.InputPer1M))
				expectedOut := int64(math.Round(float64(tokOut) * p.OutputPer1M))
				expectedThink := int64(math.Round(float64(tokThink) * p.OutputPer1M))
				expectedCacheW := int64(math.Round(float64(tokCacheW) * p.CacheWrite))
				expectedCacheR := int64(math.Round(float64(tokCacheR) * p.CacheRead))
				expected := expectedIn + expectedOut + expectedThink + expectedCacheW + expectedCacheR

				mc.ExpectedCostUUSD = expected
				mc.DriftUUSD = costUUSD - expected
				mc.Match = mc.DriftUUSD == 0
				totalExpectedUUSD += expected

				report.PerModelCheck = append(report.PerModelCheck, mc)
			}

			report.TotalRawUUSD = totalOurUUSD
			report.TotalAggregateUSD = float64(totalOurUUSD) / 1_000_000
			report.DriftUUSD = totalOurUUSD - totalExpectedUUSD
			if totalExpectedUUSD > 0 {
				report.DriftPercent = (float64(report.DriftUUSD) / float64(totalExpectedUUSD)) * 100
			}
			if modelsTotal > 0 {
				report.PricingCoverage = float64(modelsWithPricing) / float64(modelsTotal) * 100
			}

			if report.DriftUUSD == 0 && len(report.MissingModels) == 0 {
				report.Status = "balanced"
				report.Explanation = fmt.Sprintf("Perfect match across %d calls on %d models. Zero drift.", totalCalls, modelsWithPricing)
			} else if report.DriftUUSD == 0 {
				report.Status = "balanced"
				report.Explanation = fmt.Sprintf("Zero drift on priced models. %d model(s) lack pricing data.", len(report.MissingModels))
			} else {
				report.Status = "drift_detected"
				report.Explanation = fmt.Sprintf("Drift of %d µUSD detected. Likely cause: pricing table update mid-period or rounding across %d calls.", report.DriftUUSD, totalCalls)
			}

			return report, nil
		}

		// Forecast: project month-end spend + anomaly detection
		tools.OnForecastSpend = func(ctx context.Context, lookbackDays int) (tools.SpendForecast, error) {
			pool := gw.db.Pool
			now := time.Now()
			forecast := tools.SpendForecast{
				AsOf:        now.Format("2006-01-02"),
				DaysInMonth: daysInMonth(now),
				DaysElapsed: now.Day(),
			}
			forecast.DaysRemaining = forecast.DaysInMonth - forecast.DaysElapsed

			// Month-to-date total
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date >= date_trunc('month', CURRENT_DATE)`, defaultTenant).Scan(&forecast.SpentSoFarUSD)

			if forecast.DaysElapsed > 0 {
				forecast.DailyAvgUSD = forecast.SpentSoFarUSD / float64(forecast.DaysElapsed)
				forecast.ProjectedMonthUSD = forecast.DailyAvgUSD * float64(forecast.DaysInMonth)
			}

			// Trend direction: compare last 3 days avg vs previous 3 days
			var recentAvg, olderAvg float64
			pool.QueryRow(ctx,
				`SELECT COALESCE(AVG(cost_usd),0) FROM (
					SELECT SUM(cost_usd) as cost_usd FROM org_daily_spend
					WHERE tenant_id=$1 AND date >= CURRENT_DATE - 3
					GROUP BY date) t`, defaultTenant).Scan(&recentAvg)
			pool.QueryRow(ctx,
				`SELECT COALESCE(AVG(cost_usd),0) FROM (
					SELECT SUM(cost_usd) as cost_usd FROM org_daily_spend
					WHERE tenant_id=$1 AND date >= CURRENT_DATE - 6 AND date < CURRENT_DATE - 3
					GROUP BY date) t`, defaultTenant).Scan(&olderAvg)
			if olderAvg > 0 {
				ratio := recentAvg / olderAvg
				if ratio > 1.15 {
					forecast.TrendDirection = "increasing"
				} else if ratio < 0.85 {
					forecast.TrendDirection = "decreasing"
				} else {
					forecast.TrendDirection = "stable"
				}
			} else {
				forecast.TrendDirection = "stable"
			}

			// Per-agent forecast
			agentRows, err := pool.Query(ctx, `
				SELECT a.id, a.display_name, COALESCE(a.org_role,''), COALESCE(a.monthly_budget_usd,0),
				       COALESCE((SELECT SUM(cost_usd) FROM org_daily_spend WHERE agent_id=a.id AND date >= date_trunc('month', CURRENT_DATE)),0)
				FROM agents a
				WHERE a.tenant_id=$1 AND a.deleted_at IS NULL
				ORDER BY 5 DESC`, defaultTenant)
			if err == nil {
				defer agentRows.Close()
				for agentRows.Next() {
					var af tools.AgentForecast
					agentRows.Scan(&af.AgentID, &af.DisplayName, &af.OrgRole, &af.MonthlyBudget, &af.SpentThisMonth)
					if forecast.DaysElapsed > 0 {
						af.ProjectedMonth = (af.SpentThisMonth / float64(forecast.DaysElapsed)) * float64(forecast.DaysInMonth)
					}
					if af.MonthlyBudget > 0 {
						af.BudgetUtilPct = (af.SpentThisMonth / af.MonthlyBudget) * 100
						if af.ProjectedMonth > af.MonthlyBudget {
							af.WillExceedBudget = true
							af.ExceedByUSD = af.ProjectedMonth - af.MonthlyBudget
						}
					}
					forecast.AgentForecasts = append(forecast.AgentForecasts, af)
				}
			}

			// Anomaly detection: 3σ spikes per agent over lookback period
			anomalyRows, err := pool.Query(ctx, `
				SELECT a.display_name, s.date::text, s.cost_usd, a.id
				FROM org_daily_spend s JOIN agents a ON a.id = s.agent_id
				WHERE s.tenant_id=$1 AND s.date >= CURRENT_DATE - $2
				ORDER BY a.id, s.date`, defaultTenant, lookbackDays)
			if err == nil {
				defer anomalyRows.Close()
				type dailyPoint struct {
					name string
					date string
					cost float64
				}
				agentDays := map[string][]dailyPoint{}
				for anomalyRows.Next() {
					var name, date, agID string
					var cost float64
					anomalyRows.Scan(&name, &date, &cost, &agID)
					agentDays[agID] = append(agentDays[agID], dailyPoint{name: name, date: date, cost: cost})
				}
				for _, days := range agentDays {
					if len(days) < 3 {
						continue
					}
					costs := make([]float64, len(days))
					for i, d := range days {
						costs[i] = d.cost
					}
					avg := tools.Mean(costs)
					sd := tools.StdDev(costs)
					if sd == 0 {
						continue
					}
					for _, d := range days {
						sigma := (d.cost - avg) / sd
						if sigma >= 3.0 {
							forecast.Anomalies = append(forecast.Anomalies, tools.SpendAnomaly{
								AgentName:   d.name,
								Date:        d.date,
								SpendUSD:    d.cost,
								AvgUSD:      avg,
								StdDev:      sd,
								Sigma:       sigma,
								Description: fmt.Sprintf("%.1fσ above mean — investigate workload spike", sigma),
							})
						}
					}
				}
			}

			return forecast, nil
		}

		// SetBudget: CFO sets monthly/daily cap for an agent
		tools.OnSetBudget = func(ctx context.Context, agentID string, monthlyUSD, dailyUSD float64) error {
			pool := gw.db.Pool
			_, err := pool.Exec(ctx,
				`UPDATE agents SET monthly_budget_usd=$1 WHERE id=$2 AND tenant_id=$3`,
				monthlyUSD, agentID, defaultTenant)
			if err != nil {
				return err
			}
			// Also update/insert gateway_budgets for the daily cap
			if dailyUSD > 0 {
				_, err = pool.Exec(ctx, `
					INSERT INTO gateway_budgets (tenant_id, agent_id, monthly_usd, daily_usd)
					VALUES ($1, $2, $3, $4)
					ON CONFLICT (tenant_id, agent_id) WHERE agent_id IS NOT NULL
					DO UPDATE SET monthly_usd=$3, daily_usd=$4, updated_at=now()`,
					defaultTenant, agentID, monthlyUSD, dailyUSD)
			}
			return err
		}

		// RequestBudgetRaise: any agent can request a budget increase
		tools.OnRequestBudgetRaise = func(ctx context.Context, agentID, reason string, requestedUSD float64) (string, error) {
			pool := gw.db.Pool
			var currentBudget float64
			pool.QueryRow(ctx,
				`SELECT COALESCE(monthly_budget_usd,0) FROM agents WHERE id=$1`, agentID).Scan(&currentBudget)

			var id string
			err := pool.QueryRow(ctx, `
				INSERT INTO budget_requests (tenant_id, agent_id, current_usd, requested_usd, reason)
				VALUES ($1, $2, $3, $4, $5)
				RETURNING id`, defaultTenant, agentID, currentBudget, requestedUSD, reason).Scan(&id)
			if err != nil {
				return "", err
			}
			// Notify via WebSocket
			if gw.rtHub != nil {
				gw.rtHub.Broadcast(realtime.Event{
					Type: "budget_warning",
					Data: map[string]string{"type": "raise_request", "agent_id": agentID, "request_id": id},
				})
			}
			return id, nil
		}

		// ListBudgetRequests: CFO reviews all pending + recent requests
		tools.OnListBudgetRequests = func(ctx context.Context) ([]tools.BudgetRequest, error) {
			pool := gw.db.Pool
			rows, err := pool.Query(ctx, `
				SELECT br.id, br.agent_id, COALESCE(a.display_name,''), COALESCE(a.org_role,''),
				       br.current_usd, br.requested_usd, br.reason, br.status,
				       COALESCE(br.decided_by,''), COALESCE(br.decision_note,''),
				       br.created_at::text, COALESCE(br.decided_at::text,'')
				FROM budget_requests br
				LEFT JOIN agents a ON a.id = br.agent_id
				WHERE br.tenant_id=$1
				ORDER BY CASE br.status WHEN 'pending' THEN 0 ELSE 1 END, br.created_at DESC
				LIMIT 50`, defaultTenant)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var out []tools.BudgetRequest
			for rows.Next() {
				var r tools.BudgetRequest
				rows.Scan(&r.ID, &r.AgentID, &r.AgentName, &r.OrgRole,
					&r.CurrentUSD, &r.RequestedUSD, &r.Reason, &r.Status,
					&r.DecidedBy, &r.DecisionNote, &r.CreatedAt, &r.DecidedAt)
				out = append(out, r)
			}
			return out, nil
		}

		// DecideBudgetRequest: CFO approves or denies a raise
		tools.OnDecideBudgetRequest = func(ctx context.Context, requestID, decision, note string) error {
			pool := gw.db.Pool
			deciderID := tools.AgentIDFromCtx(ctx)

			// Update the request
			_, err := pool.Exec(ctx, `
				UPDATE budget_requests SET status=$1, decided_by=$2, decision_note=$3, decided_at=now()
				WHERE id=$4 AND tenant_id=$5 AND status='pending'`,
				decision, deciderID, note, requestID, defaultTenant)
			if err != nil {
				return err
			}

			// If approved, update the agent's budget
			if decision == "approved" {
				var agentID string
				var requestedUSD float64
				pool.QueryRow(ctx,
					`SELECT agent_id, requested_usd FROM budget_requests WHERE id=$1`, requestID).Scan(&agentID, &requestedUSD)
				if agentID != "" {
					pool.Exec(ctx, `UPDATE agents SET monthly_budget_usd=$1 WHERE id=$2`, requestedUSD, agentID)
				}
			}
			return nil
		}

		// CFOReport: comprehensive financial report combining all data
		tools.OnCFOReport = func(ctx context.Context, days int) (tools.CFOReportData, error) {
			pool := gw.db.Pool
			report := tools.CFOReportData{
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				Period:      time.Now().Format("January 2006"),
			}

			// Get totals
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date >= date_trunc('month', CURRENT_DATE)`, defaultTenant).Scan(&report.TotalMonthUSD)
			pool.QueryRow(ctx,
				`SELECT COALESCE(SUM(cost_usd),0) FROM org_daily_spend
				 WHERE tenant_id=$1 AND date = CURRENT_DATE`, defaultTenant).Scan(&report.TotalTodayUSD)

			// Run reconciliation
			if tools.OnReconcile != nil {
				recon, err := tools.OnReconcile(ctx)
				if err == nil {
					report.Reconciliation = recon
				}
			}

			// Run forecast
			if tools.OnForecastSpend != nil {
				fc, err := tools.OnForecastSpend(ctx, days)
				if err == nil {
					report.Forecast = fc
				}
			}

			// Budget status per agent
			budgetRows, err := pool.Query(ctx, `
				SELECT a.display_name, COALESCE(a.org_role,''), COALESCE(a.monthly_budget_usd,0),
				       COALESCE((SELECT SUM(cost_usd) FROM org_daily_spend WHERE agent_id=a.id AND date >= date_trunc('month', CURRENT_DATE)),0)
				FROM agents a
				WHERE a.tenant_id=$1 AND a.deleted_at IS NULL AND a.monthly_budget_usd > 0
				ORDER BY a.monthly_budget_usd DESC`, defaultTenant)
			if err == nil {
				defer budgetRows.Close()
				for budgetRows.Next() {
					var bs tools.AgentBudgetStatus
					budgetRows.Scan(&bs.DisplayName, &bs.OrgRole, &bs.MonthlyBudget, &bs.SpentUSD)
					if bs.MonthlyBudget > 0 {
						bs.PercentUsed = (bs.SpentUSD / bs.MonthlyBudget) * 100
					}
					switch {
					case bs.PercentUsed >= 100:
						bs.Status = "exceeded"
					case bs.PercentUsed >= 80:
						bs.Status = "critical"
					case bs.PercentUsed >= 60:
						bs.Status = "warning"
					default:
						bs.Status = "healthy"
					}
					report.BudgetStatus = append(report.BudgetStatus, bs)
				}
			}

			// Pending budget requests
			if tools.OnListBudgetRequests != nil {
				allReqs, err := tools.OnListBudgetRequests(ctx)
				if err == nil {
					for _, r := range allReqs {
						if r.Status == "pending" {
							report.PendingRequests = append(report.PendingRequests, r)
						}
					}
				}
			}

			// Generate recommendations
			for _, bs := range report.BudgetStatus {
				if bs.Status == "exceeded" {
					report.Recommendations = append(report.Recommendations,
						fmt.Sprintf("URGENT: %s has exceeded budget (%.0f%%). Suspend non-critical tasks or increase budget.", bs.DisplayName, bs.PercentUsed))
				} else if bs.Status == "critical" {
					report.Recommendations = append(report.Recommendations,
						fmt.Sprintf("WARNING: %s at %.0f%% budget utilization. Review workload or plan increase.", bs.DisplayName, bs.PercentUsed))
				}
			}
			for _, af := range report.Forecast.AgentForecasts {
				if af.WillExceedBudget {
					report.Recommendations = append(report.Recommendations,
						fmt.Sprintf("FORECAST: %s projected to exceed budget by $%.2f. Consider preemptive raise.", af.DisplayName, af.ExceedByUSD))
				}
			}
			if len(report.Reconciliation.MissingModels) > 0 {
				report.Recommendations = append(report.Recommendations,
					fmt.Sprintf("PRICING: %d model(s) lack pricing data. Add rates for accurate accounting.", len(report.Reconciliation.MissingModels)))
			}
			if report.Reconciliation.DriftUUSD != 0 {
				report.Recommendations = append(report.Recommendations,
					fmt.Sprintf("AUDIT: Cost drift of %d µUSD detected. Investigate pricing table changes.", report.Reconciliation.DriftUUSD))
			}

			return report, nil
		}
	}

	reg.Register(tools.NewHRManageTool())
	reg.Register(tools.NewFleetStatusTool())
	reg.Register(tools.NewOrgFinanceTool())
	reg.Register(tools.NewReconcileTool())
	reg.Register(tools.NewForecastSpendTool())
	reg.Register(tools.NewSetBudgetTool())
	reg.Register(tools.NewRequestBudgetRaiseTool())
	reg.Register(tools.NewListBudgetRequestsTool())
	reg.Register(tools.NewDecideBudgetRequestTool())
	reg.Register(tools.NewCFOReportTool())
	reg.Register(emailSend)
	reg.Register(emailRead)

	// Browser automation tool — AI agents can browse the web

	reg.Register(browser.NewBrowserTool(browserMgr))
	slog.Info("browser tool registered")

	// Rooms — agents can post, list, decide, and assign tasks autonomously
	{
		apiBase := "http://localhost:8486"
		var roomToken func() string
		if gw.cfg != nil && gw.cfg.Auth.Token != "" {
			tok := gw.cfg.Auth.Token
			roomToken = func() string { return tok }
		}
		reg.Register(tools.NewRoomPostToolWithAuth(apiBase, roomToken))
		reg.Register(tools.NewRoomListTool(apiBase, roomToken))
		reg.Register(tools.NewRoomDecideTool(apiBase, roomToken))
		reg.Register(tools.NewRoomAssignTool(apiBase, roomToken))
	}
	var roomMgr *tools.RoomManager
	if gw.db != nil {
		roomMgr = tools.NewRoomManager(gw.db.Pool)
	}
	reg.Register(tools.NewJoinRoomTool(roomMgr))
	reg.Register(tools.NewLeaveRoomTool(roomMgr))

	// Media
	reg.Register(tools.NewReadImageTool(gw.providerReg))
	reg.Register(tools.NewCreateImageTool(gw.providerReg, gw.mediaMgr))
	// Real PDF/DOCX extraction — replaces the earlier stub. Same
	// allow-list as read_file so both tools see the same paths.
	readDocTool := tools.NewReadDocumentV2Tool(workspace)
	readDocTool.AllowPaths(dataDir, "/tmp")
	reg.Register(readDocTool)
	// PDF quote/invoice generation — pure-Go, no system dependencies.
	reg.Register(tools.NewQuoteGenTool(workspace))
	reg.Register(tools.NewTTSTool(gw.voiceMgr))
	reg.Register(tools.NewReadAudioTool(gw.voiceMgr))
	reg.Register(tools.NewCreateAudioTool(gw.voiceMgr))
	reg.Register(tools.NewReadVideoTool(gw.providerReg))
	reg.Register(tools.NewCreateVideoTool(gw.mediaMgr))
	reg.Register(tools.NewScrapeTool())
	reg.Register(tools.NewCrawlTool())
	reg.Register(tools.NewSocialMonitorTool())

	// Workspace builder tool — Prime (and any agent) can build/modify workspaces through conversation.
	// This is what makes "tell Prime to build me a CRM" work end-to-end.
	{
		apiBase := "http://localhost:8486"
		if gw.cfg != nil && gw.cfg.Auth.Token != "" {
			tok := gw.cfg.Auth.Token
			reg.Register(tools.NewWorkspaceBuilderTool(apiBase, func() string { return tok }))
		} else {
			reg.Register(tools.NewWorkspaceBuilderTool(apiBase, nil))
		}
	}

	// Social publishing tool — lets agents create, schedule, and publish posts
	if gw.db != nil {
		socialStore := socialqor.NewStore(gw.db.Pool)
		reg.Register(socialqor.NewSocialTool(socialStore))
		// Social relay management tool — COO/agent can manage relay providers and accounts conversationally
		if gw.socialRelayStore != nil {
			reg.Register(socialqor.NewSocialRelayTool(gw.socialRelayStore, socialStore, gw.db.Pool))
			slog.Info("manage_social_relay tool registered")
		}
		// Start the social post scheduler daemon
		go gw.runSocialScheduler(socialStore)
	}
	reg.Register(tools.NewQorvenFly())                            // Qorven-Fly: flight search plugin
	reg.Register(tools.NewQorvenDownload(workspace))              // Qorven-Download: file downloader
	reg.Register(tools.NewQorvenWiki(gw.memStore, defaultTenant)) // Qorven-Wiki: knowledge base compiler
	reg.Register(tools.NewQorvenLint())                           // Qorven-Lint: health checks
	reg.Register(tools.NewQorvenReport())                         // Qorven-Report: structured outputs
	reg.Register(tools.NewResearchTool(searchPipeline))

	// Messaging + Teams
	reg.Register(tools.NewMessageTool())
	reg.Register(tools.NewSpawnTool())
	var teamTasksBackend tools.TeamTasksBackend
	if gw.taskStore != nil {
		teamTasksBackend = &taskStoreAdapter{store: gw.taskStore}
	}
	var teamMsgRuntime tools.TeamMessageRuntime
	if gw.runtimeMgr != nil {
		teamMsgRuntime = &runtimeMgrAdapter{mgr: gw.runtimeMgr}
	}
	reg.Register(tools.NewTeamTasksTool(teamTasksBackend, defaultTenant))
	reg.Register(tools.NewTeamMessageTool(teamTasksBackend, teamMsgRuntime))

	// ── GitHub Tools — autonomous dev loop ──────────────────────────────────
	// Token lookup: env GITHUB_TOKEN → vault credential "github" → empty (tools return helpful error).
	{
		ghToken := os.Getenv("GITHUB_TOKEN")
		if ghToken == "" && gw.vault != nil {
			if cred, err := gw.vault.Get(context.Background(), defaultTenant, "github"); err == nil {
				if cred.Data.APIKey != "" {
					ghToken = cred.Data.APIKey
				} else if cred.Data.AccessToken != "" {
					ghToken = cred.Data.AccessToken
				}
			}
		}
		// Inject token into tool context via a middleware registered on the agent loop.
		// Tools call ghTokenFromCtx(ctx) — the loop injects it before each tool execution.
		if ghToken != "" {
			tools.AddDynamicScrubValues(ghToken) // prevent token leaking in tool output
			slog.Info("github.tools.configured")
		} else {
			slog.Info("github.tools.no_token — set GITHUB_TOKEN or add via Settings → Provider Keys")
		}
		// Register the token getter so tools can read it at call time (supports hot-reload).
		ghGetToken := func() string {
			// Re-read each call so vault updates take effect without restart.
			if t := os.Getenv("GITHUB_TOKEN"); t != "" {
				return t
			}
			if gw.vault != nil {
				if cred, err := gw.vault.Get(context.Background(), defaultTenant, "github"); err == nil {
					if cred.Data.APIKey != "" {
						return cred.Data.APIKey
					}
					if cred.Data.AccessToken != "" {
						return cred.Data.AccessToken
					}
				}
			}
			return ""
		}
		reg.Register(tools.NewGhRepoInfoToolWithToken(ghGetToken))
		reg.Register(tools.NewGhListIssuesToolWithToken(ghGetToken))
		reg.Register(tools.NewGhReadIssueToolWithToken(ghGetToken))
		reg.Register(tools.NewGhCreateIssueToolWithToken(ghGetToken))
		reg.Register(tools.NewGhCreateBranchToolWithToken(ghGetToken))
		// gh_push_file writes code to a user's GitHub repo — the single
		// most destructive tool in the GH tool family. Wrap it with the
		// permission gate so every call requires explicit user consent.
		// WrapLazy defers the gate lookup to Execute-time because the
		// gate is constructed in ensureProtocolSurfaces (later in boot).
		reg.Register(permissions.WrapLazy(
			func() *permissions.Gate { return gw.permissionGate },
			tools.NewGhPushFileToolWithToken(ghGetToken),
			permissions.GatedToolOptions{
				Reason:      "Writes a file to a user-owned GitHub repository",
				RequestedBy: "agent",
				// SessionIDFromArgs defaults to args["session_id"]; tool
				// runner must populate it. Default suffices today.
			},
		))
		reg.Register(tools.NewGhOpenPRToolWithToken(ghGetToken))
		reg.Register(tools.NewGhPostCommentToolWithToken(ghGetToken))
		reg.Register(tools.NewGhListPRChecksToolWithToken(ghGetToken))
		reg.Register(tools.NewGhMergePRToolWithToken(ghGetToken))
		reg.Register(tools.NewGhCreateRepoToolWithToken(ghGetToken))

		// gh_task_register — agent commits to working on an issue autonomously.
		// Wire the global task queue via callback (avoids circular tools→agent import).
		tools.SetGitHubTaskRegisterFn(func(agentID, owner, repo, branch, roomID string, issueNum int) string {
			return agent.GlobalGitHubTaskQueue.Register(agentID, owner, repo, issueNum, branch, roomID)
		})
		reg.Register(tools.NewGhTaskRegisterTool())
	}

	// Skills
	gw.skillLoader = skills.NewLoader(workspace, "", "skills")
	reg.Register(tools.NewSkillSearchTool(gw.skillLoader))
	reg.Register(tools.NewUseSkillTool(gw.skillLoader))
	skillManage := tools.NewSkillManageTool(workspace, gw.skillLoader)
	reg.Register(skillManage)
	// Wire pin-check callback so skill_manage is fail-closed on pinned skills.
	if gw.skillStore != nil {
		tools.OnSkillIsPinned = func(ctx context.Context, slug string) bool {
			return gw.skillStore.IsPinnedBySlug(ctx, defaultTenant, slug)
		}
	}

	// Custom tools from DB
	if gw.customTools != nil {
		if err := gw.customTools.LoadAndRegister(context.Background(), defaultTenant, workspace, reg); err != nil {
			slog.Warn("failed to load custom tools", "error", err)
		}
	}

	// Register connector registry + 12 gold connectors
	gw.connReg = connectors.NewRegistry()
	connectors.RegisterAll(gw.connReg)

	// Register execute_action tool (native connector executor)
	if gw.connExec != nil {
		reg.Register(connectors.NewExecuteActionTool(gw.connExec))
		slog.Info("execute_action tool registered")
	}

	// Register list_connected_platforms tool for agent awareness
	if gw.relayStore != nil && gw.connKB != nil {
		awareness := connectors.NewPlatformAwareness(gw.relayStore, gw.connKB)
		reg.Register(connectors.NewConnectedPlatformsTool(awareness))
		slog.Info("list_connected_platforms tool registered")
	}
	slog.Info("connectors registered", "count", len(gw.connReg.List()))

	slog.Info("tools registered", "count", reg.Count())

	// Delegate tool: allows Prime to send tasks to specialist agents
	delegateTool := tools.NewDelegateTool(
		func(ctx context.Context, agentKey, message string) (string, error) {
			return gw.agentLoop.Chat(ctx, agentKey, message)
		},
		func(ctx context.Context) ([]map[string]any, error) {
			if gw.agents == nil {
				return nil, nil
			}
			agents, err := gw.agents.List(ctx, defaultTenant)
			if err != nil {
				return nil, err
			}
			result := []map[string]any{}
			for _, a := range agents {
				result = append(result, map[string]any{
					"key": a.AgentKey, "name": a.DisplayName, "role": func() string {
						if a.Role != nil {
							return *a.Role
						}
						return ""
					}(),
					"model": a.Model,
				})
			}
			return result, nil
		},
	)
	// Background job completion: Prime hears the result on its next turn via supervisor bus
	if gw.agentLoop != nil && gw.agentLoop.SupervisorBus != nil && gw.agentLoop.PrimeID != "" {
		primeID := gw.agentLoop.PrimeID
		bus := gw.agentLoop.SupervisorBus
		delegateTool.OnComplete = func(jobID, agentKey, result string, err error) {
			content := fmt.Sprintf("[BACKGROUND_JOB_DONE:%s] %s finished. Result:\n%s", jobID, agentKey, result)
			if err != nil {
				content = fmt.Sprintf("[BACKGROUND_JOB_DONE:%s] %s failed: %v", jobID, agentKey, err)
			}
			bus.Send(context.Background(), supervisorpkg.Message{
				From:    agentKey,
				To:      primeID,
				Intent:  supervisorpkg.IntentHeartbeat,
				Content: content,
			})
		}
	}
	reg.Register(delegateTool)
	reg.Register(tools.NewListAgentsTool(func(ctx context.Context) ([]map[string]any, error) {
		if gw.agents == nil {
			return nil, nil
		}
		agents, err := gw.agents.List(ctx, defaultTenant)
		if err != nil {
			return nil, err
		}
		result := []map[string]any{}
		for _, a := range agents {
			result = append(result, map[string]any{
				"key": a.AgentKey, "name": a.DisplayName, "role": func() string {
					if a.Role != nil {
						return *a.Role
					}
					return ""
				}(),
			})
		}
		return result, nil
	}))
	slog.Info("delegation tools registered")

	// QOROS tools — sleep + daily_log
	qorosLookup := func(agentID string) *agent.QorosMode {
		if gw.brain == nil {
			return nil
		}
		return gw.brain.Qoros[agentID]
	}
	reg.Register(agent.NewSleepTool(qorosLookup))
	reg.Register(agent.NewDailyLogTool(qorosLookup))
	reg.Register(agent.NewProjectTool("/tmp/qorven-projects"))
	reg.Register(agent.NewPrimeCoderTool(gw.projectReg))
	// Self-improvement tools — point at the backend source tree when
	// available so the self_improve / self_patch paths can read their
	// own code. Users override via QORVEN_SELF_REPO; the default uses
	// the current working directory, which is correct for `qorven
	// start` run from the repo root or from a binary install where
	// these tools simply no-op on missing paths.
	selfRepo := os.Getenv("QORVEN_SELF_REPO")
	if selfRepo == "" {
		if cwd, err := os.Getwd(); err == nil {
			selfRepo = cwd
		}
	}
	reg.Register(agent.NewSelfKnowledgeTool(selfRepo))
	reg.Register(agent.NewSelfPatchTool(selfRepo))
	reg.Register(tools.NewSelfTest(selfRepo))
	reg.Register(tools.NewSelfImprove(selfRepo))
	reg.Register(tools.NewManageAgents())

	// spawn_team — budget- and timeline-aware team provisioning for Prime.
	{
		modelForTier := func(tier string) string {
			if gw.agentLoop == nil || gw.agentLoop.SmartRouter == nil {
				return ""
			}
			return gw.agentLoop.SmartRouter.BestModelForTier(tier)
		}
		tools.OnModelForTier = modelForTier
		if tools.OnAgentCreate != nil {
			reg.Register(tools.NewSpawnTeam(tools.OnAgentCreate, modelForTier))
		}
	}

	// Native flight search.
	reg.Register(tools.NewFlightSearchTool())

	// Shipment tracking — DHL, FedEx, SF Express, YTO, STO, Best Express.
	// Keys stored in provider_keys with category "tracking". Key format:
	//   dhl                → plain API key string
	//   fedex              → "client_id:client_secret" (colon-separated)
	//   sf_express/yto/sto → "app_id:app_key" (colon-separated)
	//   best               → "app_id:app_secret" (colon-separated)
	{
		trackGetKey := func(carrier string) string {
			// Env var fallback (TRACKING_DHL_KEY, TRACKING_FEDEX_KEY, …)
			envKey := "TRACKING_" + strings.ToUpper(carrier) + "_KEY"
			if v := os.Getenv(envKey); v != "" {
				return v
			}
			if gw.db != nil {
				ks := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
				keys, _ := ks.ListKeys(context.Background(), defaultTenant, carrier)
				for _, k := range keys {
					if k.Status == "verified" {
						if dk, err := providers.DecryptKeyBytes(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey); err == nil {
							return string(dk)
						}
					}
				}
			}
			return ""
		}
		reg.Register(tools.NewTrackShipmentTool(trackGetKey))
		slog.Info("track_shipment tool registered")
	}

	// Store credential tool — lets agents save API keys into the encrypted vault
	// so connector binaries receive them via CONNECTOR_<SLUG>_KEY at run time.
	if gw.db != nil {
		ks := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
		reg.Register(tools.NewStoreCredentialTool(ks, defaultTenant))
		slog.Info("store_credential tool registered")

		// set_rule tool — lets Prime create background rules from user-stated policies.
		reg.Register(tools.NewSetRuleTool(gw.db.Pool, defaultTenant))
		slog.Info("set_rule tool registered")
	}

	// Connector template tool — returns ready-to-adapt Go source for REST GET,
	// REST POST, and RSS connectors. No dependencies; always registered.
	reg.Register(&tools.GetConnectorTemplateTool{})
	slog.Info("get_connector_template tool registered")

	// Carrier integration tools — scaffold shipping carrier connectors.
	reg.Register(tools.NewBuildCarrierTool())
	reg.Register(tools.NewListCarriersTool())
	slog.Info("carrier tools registered", "tools", "build_carrier,list_carriers")

	// Coding tools.
	fileHistory := tools.NewFileHistory()
	projectReg := tools.NewProjectRegistry(config.DataDir())
	gw.projectReg = projectReg
	reg.Register(tools.NewGlobTool(workspace))
	reg.Register(tools.NewGrepTool(workspace))
	reg.Register(tools.NewDiagnosticsTool())
	reg.Register(tools.NewApplyPatchTool(workspace, fileHistory))
	reg.Register(tools.NewUndoTool(fileHistory))
	reg.Register(tools.NewProjectManagerTool(projectReg))
	slog.Info("coding tools registered", "tools", "glob,grep,diagnostics,apply_patch,undo,project_manager")

	// Git tools — status, diff, log.
	reg.Register(tools.NewGitStatusTool())
	reg.Register(tools.NewGitDiffTool())
	reg.Register(tools.NewGitLogTool())

	// Background job tools — spawn/output/kill/list long-running processes.
	reg.Register(tools.NewJobSpawnTool(workspace))
	reg.Register(tools.NewJobOutputTool())
	reg.Register(tools.NewJobKillTool())
	reg.Register(tools.NewJobListTool())

	// Session TODO tools — in-session task tracking.
	reg.Register(tools.NewTodoWriteTool())
	reg.Register(tools.NewTodoReadTool())

	// Multi-edit — atomic multi-file write.
	reg.Register(tools.NewMultiEditTool(workspace))

	// CLI agent adapter — delegate coding tasks to claude/codex/kilo CLIs.
	reg.Register(tools.NewRunCLIAgentTool(workspace))

	// Browser-LLM autonomous loop (browse-agent pattern)
	browseAgent := agent.NewBrowseAgent(browserMgr, gw.providerReg.Default(), "")
	reg.Register(agent.NewBrowseTool(browseAgent))

	// Sandbox app runner tools — run/list/stop Docker containers on behalf of agents.
	if gw.appRunner != nil {
		reg.Register(tools.NewRunAppTool(func(ctx context.Context, p tools.RunAppParams) (*tools.RunningAppResult, error) {
			ra, err := gw.appRunner.Start(ctx, sandbox.RunAppParams{
				TenantID:    p.TenantID,
				SessionID:   p.SessionID,
				AgentID:     p.AgentID,
				ImageOrRepo: p.ImageOrRepo,
				Port:        p.Port,
				Label:       p.Label,
				TTLMinutes:  p.TTLMinutes,
				Env:         p.Env,
			})
			if err != nil {
				return nil, err
			}
			return &tools.RunningAppResult{
				ID:          ra.ID,
				ContainerID: ra.ContainerID,
				Image:       ra.Image,
				Label:       ra.Label,
				ProxyPrefix: ra.ProxyPrefix,
				ProxyURL:    ra.ProxyURL,
				Status:      ra.Status,
				HostPort:    ra.HostPort,
				ExpiresAt:   ra.ExpiresAt,
			}, nil
		}))
		reg.Register(tools.NewListRunningAppsTool(func(ctx context.Context, tenantID string) ([]tools.RunningAppResult, error) {
			apps, err := gw.appRunner.List(ctx, tenantID)
			if err != nil {
				return nil, err
			}
			var result []tools.RunningAppResult
			for _, a := range apps {
				result = append(result, tools.RunningAppResult{
					ID:          a.ID,
					ContainerID: a.ContainerID,
					Image:       a.Image,
					Label:       a.Label,
					ProxyPrefix: a.ProxyPrefix,
					ProxyURL:    a.ProxyURL,
					Status:      a.Status,
					HostPort:    a.HostPort,
					ExpiresAt:   a.ExpiresAt,
				})
			}
			return result, nil
		}))
		reg.Register(tools.NewStopAppTool(func(ctx context.Context, tenantID, id string) error {
			return gw.appRunner.Stop(ctx, id, tenantID)
		}))
		slog.Info("sandbox app runner tools registered")
	}

	// Dashboard tile tools — pin/unpin data tiles driven by connector snapshots.
	if gw.db != nil && gw.tileStore != nil {
		reg.Register(tools.NewPinToDashboardTool(
			func(ctx context.Context, t tools.PinnedTileInput) (string, error) {
				created, err := gw.tileStore.Create(ctx, dashboard.PinnedTile{
					TenantID:           defaultTenant,
					SourceSlug:         t.SourceSlug,
					ToolName:           t.ToolName,
					ToolArgs:           t.ToolArgs,
					WidgetType:         t.WidgetType,
					Label:              t.Label,
					Position:           t.Position,
					RefreshIntervalSec: t.RefreshIntervalSec,
				})
				if err != nil {
					return "", err
				}
				return created.ID, nil
			},
			defaultTenant,
		))
		reg.Register(tools.NewUnpinFromDashboardTool(
			func(ctx context.Context, tenantID, id string) error {
				return gw.tileStore.Delete(ctx, tenantID, id)
			},
			defaultTenant,
		))
		slog.Info("dashboard tile tools registered")
	}

	// Mail tools — always register if DB is present
	if gw.db != nil {
		reg.Register(tools.NewSetMailRuleTool(gw.db.Pool))
		reg.Register(tools.NewSetMailPolicyTool(gw.db.Pool))
		slog.Info("mail tools registered")
	}
}

func (gw *Gateway) loadProvidersFromDB() {
	if gw.providerStore == nil {
		return
	}
	keyStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)

	// 1. Load explicitly configured providers
	configs, err := gw.providerStore.ListWithKeys(context.Background(), defaultTenant)
	if err != nil {
		slog.Warn("failed to load providers from DB", "error", err)
	} else if len(configs) > 0 {
		// Backfill API key from provider_keys or OAuth token manager.
		for i, cfg := range configs {
			if cfg.APIKey != "" {
				continue
			}
			// OAuth-connected providers: fetch live token from oauth_tokens table.
			if cfg.OAuthProvider != "" && gw.llmOAuthMgr != nil {
				if tok, err := gw.llmOAuthMgr.Token(context.Background(), defaultTenant, cfg.OAuthProvider); err == nil && tok != "" {
					configs[i].APIKey = tok
					slog.Info("provider.oauth_token_injected", "provider", cfg.Name, "oauth", cfg.OAuthProvider)
					continue
				}
			}
			// cfg.ID is the providers.id UUID — look up verified key by that UUID
			keys, _ := keyStore.ListKeys(context.Background(), defaultTenant, cfg.ID)
			for _, k := range keys {
				if k.Status == "verified" {
					decrypted, _ := providers.DecryptKeyBytes(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey)
					if decrypted != "" {
						configs[i].APIKey = decrypted
						break
					}
				}
			}
		}
		if err := gw.providerReg.LoadAll(configs); err != nil {
			slog.Warn("failed to register DB providers", "error", err)
		}
		slog.Info("providers loaded from database", "count", len(configs))
	}

	// 2. Auto-register providers from provider_keys or OAuth tokens (Models Hub keys)
	// If a user added Gemini keys via Models Hub but no providers row exists,
	// create a provider instance using the catalog defaults + first available key.
	catalog := providers.ProviderCatalog()
	for _, manifest := range catalog {
		// Skip if already registered
		if _, ok := gw.providerReg.GetByName(manifest.ID); ok {
			continue
		}

		// For OAuth providers, try the OAuth token manager first (no key pool needed).
		if manifest.AuthType == "oauth2" && gw.llmOAuthMgr != nil {
			if tok, err := gw.llmOAuthMgr.Token(context.Background(), defaultTenant, manifest.ID); err == nil && tok != "" {
				cfg := providers.ProviderConfig{
					ID:            "oauth-" + manifest.ID,
					Name:          manifest.ID,
					DisplayName:   manifest.Name,
					ProviderType:  manifest.DriverType,
					APIBase:       manifest.DefaultAPIBase,
					APIKey:        tok,
					OAuthProvider: manifest.ID,
					Enabled:       true,
				}
				if err := gw.providerReg.Register(cfg); err == nil {
					slog.Info("provider auto-registered from oauth", "provider", manifest.ID)
				}
				continue
			}
		}

		keys, _ := keyStore.ListKeys(context.Background(), defaultTenant, manifest.ID)
		if len(keys) == 0 {
			continue
		}
		// Find first verified key
		var apiKey string
		for _, k := range keys {
			if k.Status == "verified" {
				apiKey, _ = providers.DecryptKeyBytes(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey)
				break
			}
		}
		if apiKey == "" {
			continue
		}
		// Determine provider type from catalog
		provType := manifest.Category
		if provType == "openai_compatible" {
			provType = providers.TypeOpenAICompat
		}
		// Skip search-only providers from chat routing
		if manifest.ID == "perplexity" {
			continue
		}
		cfg := providers.ProviderConfig{
			ID:           "keypool-" + manifest.ID,
			Name:         manifest.ID,
			DisplayName:  manifest.Name,
			ProviderType: provType,
			APIBase:      manifest.DefaultAPIBase,
			APIKey:       apiKey,
			Enabled:      true,
		}
		if err := gw.providerReg.Register(cfg); err != nil {
			slog.Warn("failed to auto-register provider from keys", "provider", manifest.ID, "error", err)
		} else {
			slog.Info("provider auto-registered from keys", "provider", manifest.ID, "model", manifest.DefaultModel)
		}
	}

	// Validate: ensure at least one provider works
	defProv := gw.providerReg.Default()
	if defProv == nil {
		slog.Error("NO LLM PROVIDER CONFIGURED — agents will not be able to respond. Add a provider in Settings → Models Hub or set one in config.toml")
	} else {
		// Quick health check: try a minimal chat using the provider's own default model
		hctx := providers.WithMeterScope(context.Background(), providers.MeterScope{TenantID: defaultTenant, Origin: providers.OriginSystem})
		testResp, testErr := defProv.Chat(hctx, providers.ChatRequest{
			Model: defProv.DefaultModel(), Messages: []providers.Message{{Role: "user", Content: "hi"}},
			Options: map[string]any{"max_tokens": 1},
		})
		if testErr != nil {
			slog.Warn("LLM provider health check failed — agents may not respond", "error", testErr)
		} else {
			slog.Info("LLM provider verified", "provider", defProv.Name(), "test_response", testResp.Content[:min(len(testResp.Content), 20)])
		}
	}
}

func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}
