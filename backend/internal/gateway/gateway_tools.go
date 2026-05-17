// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	socialqor "github.com/qorvenai/qorven/internal/qor/social"
	"github.com/qorvenai/qorven/internal/agent"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/dashboard"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/qor/browser"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/sandbox"
	"github.com/qorvenai/qorven/internal/scraper"
	"github.com/qorvenai/qorven/internal/search"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/storage"
	"github.com/qorvenai/qorven/internal/tools"
)

func (gw *Gateway) registerTools() {
	workspace := "/tmp/qorven-workspace" // default workspace
	reg := gw.toolReg

	// Filesystem — allow workspace + home dir + /tmp for full coding capability
	homeDir, _ := os.UserHomeDir()
	readTool := tools.NewReadFileTool(workspace)
	readTool.AllowPaths(homeDir, "/tmp")
	reg.Register(readTool)
	writeTool := tools.NewWriteFileTool(workspace)
	writeTool.AllowPaths(homeDir, "/tmp")
	reg.Register(writeTool)
	listTool := tools.NewListFilesTool(workspace)
	listTool.AllowPaths(homeDir, "/tmp")
	reg.Register(listTool)
	editTool := tools.NewEditTool(workspace)
	editTool.AllowPaths(homeDir, "/tmp")
	reg.Register(editTool)

	// Runtime
	reg.Register(tools.NewExecTool(workspace, true))

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
	digestTool.AllowPaths(homeDir, "/tmp")
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

	// Wire file → drive sync
	if gw.driveStore != nil {
		tools.OnFileWritten = func(ctx context.Context, agentID, name, path string, size int64) {
			mime := agent.MimeFromExt(filepath.Ext(path))
			gw.driveStore.CreateFile(ctx, defaultTenant, agentID, filepath.Base(name), path, mime, size, false, nil)
			// Track file in session
			if sid := tools.SessionIDFromCtx(ctx); sid != "" && gw.sessions != nil {
				gw.sessions.TrackFile(ctx, sid, path, "modified")
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
	}

	reg.Register(emailSend)
	reg.Register(emailRead)

	// Browser automation tool — AI agents can browse the web

	reg.Register(browser.NewBrowserTool(browserMgr))
	slog.Info("browser tool registered")

	// Rooms — agents can post, list, decide, and assign tasks autonomously
	{
		apiBase := "http://localhost:4200"
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
	readDocTool.AllowPaths(homeDir, "/tmp")
	reg.Register(readDocTool)
	// PDF quote/invoice generation — pure-Go, no system dependencies.
	reg.Register(tools.NewQuoteGenTool(workspace))
	reg.Register(tools.NewTTSTool(gw.voiceMgr))
	reg.Register(tools.NewReadAudioTool(gw.voiceMgr))
	reg.Register(tools.NewCreateAudioTool())
	reg.Register(tools.NewReadVideoTool())
	reg.Register(tools.NewCreateVideoTool(gw.mediaMgr))
	reg.Register(tools.NewScrapeTool())
	reg.Register(tools.NewCrawlTool())
	reg.Register(tools.NewSocialMonitorTool())

	// Workspace builder tool — Prime (and any agent) can build/modify workspaces through conversation.
	// This is what makes "tell Prime to build me a CRM" work end-to-end.
	{
		apiBase := "http://localhost:4200"
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
	reg.Register(tools.NewTeamTasksTool())
	reg.Register(tools.NewTeamMessageTool())

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
	slog.Info("connectors registered", "count", len(gw.connReg.List()))

	slog.Info("tools registered", "count", reg.Count())

	// Delegate tool: allows Prime to send tasks to specialist agents
	reg.Register(tools.NewDelegateTool(
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
	))
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

	// Native flight search.
	reg.Register(tools.NewFlightSearchTool())

	// Shipment tracking — DHL (real API), FedEx (real API), SF/YTO/STO/Best (stubs).
	// Keys stored in provider_keys with category "tracking". Key format:
	//   dhl   → plain API key string
	//   fedex → "client_id:client_secret" (colon-separated)
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
	}

	// Connector template tool — returns ready-to-adapt Go source for REST GET,
	// REST POST, and RSS connectors. No dependencies; always registered.
	reg.Register(&tools.GetConnectorTemplateTool{})
	slog.Info("get_connector_template tool registered")

	// Coding tools.
	fileHistory := tools.NewFileHistory()
	projectReg := tools.NewProjectRegistry(os.Getenv("HOME") + "/.qorven")
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
	// 1. Load explicitly configured providers
	configs, err := gw.providerStore.ListWithKeys(context.Background(), defaultTenant)
	if err != nil {
		slog.Warn("failed to load providers from DB", "error", err)
	} else if len(configs) > 0 {
		if err := gw.providerReg.LoadAll(configs); err != nil {
			slog.Warn("failed to register DB providers", "error", err)
		}
		slog.Info("providers loaded from database", "count", len(configs))
	}

	// 2. Auto-register providers from provider_keys (Models Hub keys)
	// If a user added Gemini keys via Models Hub but no providers row exists,
	// create a provider instance using the catalog defaults + first available key.
	keyStore := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
	catalog := providers.ProviderCatalog()
	for _, manifest := range catalog {
		// Skip if already registered
		if _, ok := gw.providerReg.GetByName(manifest.ID); ok {
			continue
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
		testResp, testErr := defProv.Chat(context.Background(), providers.ChatRequest{
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
