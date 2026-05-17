// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/qorvenai/qorven/internal/council"
	"github.com/qorvenai/qorven/internal/providers"
	systempkg "github.com/qorvenai/qorven/internal/system"
)

// registerV1Routes installs every authenticated /v1/* endpoint on the
// given parent router. Call via buildV1Router which ensures the
// protocol surfaces are initialized first.
//
// This is the single source of truth for the /v1 route map. gateway.New()
// and phase2 integration tests both call this function, so a refactor
// that drops an AuthMiddlewareV2 call or reorders a Mount cannot be
// silently diverged between production and the test path.
func (gw *Gateway) registerV1Routes(parent chi.Router) {
	parent.Route("/v1", func(r chi.Router) {
		r.Use(gw.AuthMiddlewareV2)
		// scope every authenticated request to its
		// tenant's Postgres transaction in multi-tenant mode. No-op in
		// single-tenant. MUST follow AuthMiddlewareV2 because it reads
		// the user from ctx.
		r.Use(gw.TenantScopeMiddleware)

		// Canonical command API — single source of truth for every
		// user-initiated action reachable from web AND tui. See
		// QORVEN-APP-BUILDER-DEEP-PLAN.md §A4.
		//
		// commands land here and can trigger plan runs,
		// so they're the primary surface a tenant can spam. The
		// per-tenant quota middleware caps concurrent plan-run
		// requests + sustained rate per the byom knobs. GETs on
		// other routes are intentionally NOT quota'd — read
		// traffic is cheap and paging is the client's problem.
		r.With(gw.TenantQuotaMiddleware).Mount("/commands", gw.cmdServer.Routes())

		// Plan graph + approvals + permission gate.
		r.Get("/plans", gw.handleListPlans)
		r.Get("/plans/archived", gw.handleListArchivedPlans)
		r.Post("/plans", gw.handleCreatePlan)
		r.Get("/plans/{id}", gw.handleGetPlan)
		r.Get("/plans/{id}/nodes", gw.handleListPlanNodes)
		r.Post("/plans/{id}/approve", gw.handleApprovePlan)
		r.Post("/plans/{id}/reject", gw.handleRejectPlan)
		r.Post("/plans/{id}/revise", gw.handleRevisePlan)
		r.Post("/plans/{id}/archive", gw.handleArchivePlan)
		r.Post("/approvals/{id}/comments", gw.handleAppendApprovalComment)
		r.Post("/permissions/{id}/reply", gw.handlePermissionReply)
		r.Get("/permissions", gw.handleListPendingPermissions)

		r.Post("/chat/completions", gw.handleChatCompletions)
		r.Get("/models", gw.handleOpenAIModels)
		r.Post("/chat/btw", gw.handleBTW)

		// AG-UI protocol endpoint — streams newline-delimited JSON events
		// per the AG-UI spec. Accepts RunAgentInput; use alongside
		// @ag-ui-protocol/client for first-class framework integrations.
		if gw.aguiHandler != nil {
			r.With(gw.TenantQuotaMiddleware).Post("/agui/stream", gw.aguiHandler.ServeHTTP)
		}

		// Scenario Lab
		if gw.scenarioHandlers != nil {
			r.Post("/scenarios", gw.scenarioHandlers.HandleCreate)
			r.Get("/scenarios", gw.scenarioHandlers.HandleList)
			r.Get("/scenarios/{id}", gw.scenarioHandlers.HandleGet)
			r.Post("/scenarios/{id}/run", gw.scenarioHandlers.HandleRun)
			r.Post("/scenarios/{id}/inject", gw.scenarioHandlers.HandleInject)
		}

		// Files
		r.Post("/files", gw.handleFileUpload)
		r.Get("/files/{id}/content", gw.handleFileContent)

		// Agents
		r.Get("/agents", gw.handleListAgents)
		r.Post("/agents", gw.handleCreateAgent)
		r.Post("/agents/generate-soul", gw.handleGenerateSoul)
		r.Get("/agents/{id}", gw.handleGetAgent)
		r.Post("/agents/{id}/subconscious", gw.handleRunSubconscious)
		r.Put("/agents/{id}", gw.handleUpdateAgent)
		r.Delete("/agents/{id}", gw.handleDeleteAgent)
		r.Get("/agents/chief", gw.handleGetChief)

		// Discussion history — list grouped conversations + label editing
		r.Get("/agents/{id}/discussions", gw.handleListDiscussions)
		r.Put("/agents/{id}/discussions/{discussionId}", gw.handleUpdateDiscussion)
		r.Get("/agents/{id}/messages", gw.handleAgentMessages)

		// Per-agent permission profiles
		r.Get("/agents/{id}/permissions", gw.handleListAgentPermissions)
		r.Put("/agents/{id}/permissions", gw.handleUpsertAgentPermission)
		r.Delete("/agents/{id}/permissions/{tool}", gw.handleDeleteAgentPermission)

		// Persistent runtime intervention (migration 071)
		r.Post("/agents/{id}/runtime/pause", gw.handleRuntimePause)
		r.Post("/agents/{id}/runtime/resume", gw.handleRuntimeResume)
		r.Post("/agents/{id}/runtime/wakeup", gw.handleRuntimeWakeup)
		r.Post("/agents/{id}/runtime/override", gw.handleRuntimeOverride)
		r.Get("/runtime/states", gw.handleRuntimeStates)

		// Sessions
		r.Get("/sessions", gw.handleListSessions)
		r.Get("/sessions/unified", gw.handleUnifiedTimeline) // merged cross-channel timeline for one agent
		r.Get("/sessions/search", gw.handleSearchSessions)
		r.Get("/sessions/export", gw.handleExportTrajectory)
		r.Post("/sessions", gw.handleCreateSession)
		r.Get("/sessions/{id}", gw.handleGetSession)
		r.Delete("/sessions/{id}", gw.handleDeleteSession)
		r.Get("/sessions/{id}/files", gw.handleGetSessionFiles)
		r.Get("/sessions/{id}/messages", gw.handleGetSessionMessages)
		r.Post("/sessions/{id}/messages", gw.handleAddSessionMessage)
		r.Delete("/sessions/{id}/messages", gw.handleDeleteSessionMessage)

		// Traces & observability
		r.Get("/traces", gw.handleListTraces)
		r.Get("/traces/summary", gw.handleGetTraceSummary)
		r.Get("/traces/{id}", gw.handleGetTrace)
		r.Get("/traces/{id}/spans", gw.handleListTraceSpans)

		// Projects (code workspaces)
		r.Get("/projects", gw.handleListProjects)
		r.Post("/projects", gw.handleCreateProject)
		r.Get("/projects/{id}", gw.handleGetProject)
		r.Delete("/projects/{id}", gw.handleDeleteProject)
		r.Post("/projects/{id}/tasks", gw.handleAddProjectTask)
		r.Put("/projects/{id}/tasks/{taskId}", gw.handleToggleProjectTask)
		r.Put("/projects/{id}/notes", gw.handleUpdateProjectNotes)
		r.Post("/projects/{id}/build", gw.handleBuildProject)
		// GitHub connect — per-project repo binding + webhook secret
		r.Post("/projects/{id}/github/connect", gw.handleProjectGitHubConnect)
		r.Delete("/projects/{id}/github/connect", gw.handleProjectGitHubDisconnect)
		r.Get("/projects/{id}/github/status", gw.handleProjectGitHubStatus)
		r.Post("/projects/{id}/approve", gw.handleApproveProject)
		r.Get("/projects/{id}/stream", gw.handleProjectBuildStream)
		r.Get("/projects/{id}/tree", gw.handleProjectTree)
		r.Get("/projects/{id}/file", gw.handleReadProjectFile)
		r.Put("/projects/{id}/file", gw.handleWriteProjectFile)

		// Providers
		r.Get("/providers", gw.handleListProviders)
		r.Post("/providers", gw.handleCreateProviderDB)
		r.Get("/providers/{id}", gw.handleGetProvider)
		r.Put("/providers/{id}", gw.handleUpdateProvider)
		r.Delete("/providers/{id}", gw.handleDeleteProvider)
		r.Post("/providers/{id}/verify", gw.handleVerifyProvider)
		r.Patch("/providers/{id}/capabilities", gw.handleUpdateProviderCapabilities)
		r.Post("/providers/test", gw.handleTestProvider)
		r.Get("/providers/{id}/models", gw.handleListProviderModels)
		r.Get("/providers/catalog", gw.handleProviderCatalog)
		r.Get("/providers/model-registry", gw.handleModelRegistry)
		r.Post("/providers/probe-models", gw.handleProbeModels)
		r.Get("/providers/{provider_id}/keys", gw.handleListProviderKeys)
		r.Post("/providers/{provider_id}/keys", gw.handleAddProviderKey)
		r.Post("/providers/keys/{key_id}/verify", gw.handleVerifyProviderKey)
		r.Delete("/providers/keys/{key_id}", gw.handleRetireProviderKey)
		r.Put("/providers/keys/{key_id}/budget", gw.handleSetKeyBudget)
		r.Post("/providers/keys/{key_id}/test", gw.handleTestKeyAndFetchModels)
		r.Get("/providers/{provider_id}/pool", gw.handleGetPoolConfig)
		r.Put("/providers/{provider_id}/pool", gw.handleSavePoolConfig)
		r.Get("/providers/{provider_id}/usage", gw.handleKeyUsageLogs)
		r.Get("/providers/{provider_id}/live-models", gw.handleFetchLiveModels)
		r.Get("/models/catalog", gw.handleModelCatalog)
		r.Get("/models/recommended", gw.handleRecommendedModels)
		r.Get("/models/available", gw.handleGetAvailableModels)
		r.Get("/models/selected", gw.handleListSelectedModels)
		r.Post("/models/select", gw.handleSelectModel)
		r.Delete("/models/select", gw.handleDeselectModel)
		r.Post("/models/default", gw.handleSetDefaultModel)
		r.Get("/models/discovered", gw.handleListDiscoveredModels)
		r.Post("/models/discovered/{id}/{action}", gw.handleActionDiscoveredModel)

		// Search Providers — key management for web-search grounding
		r.Get("/system/search-providers", gw.handleGetSearchProviders)
		r.Post("/system/search-providers", gw.handleSaveSearchProvider)
		r.Get("/system/integrations", gw.handleGetIntegrations)
		r.Post("/system/integrations", gw.handleSaveIntegration)

		// Usage & Budget
		r.Get("/usage/soul/{soul_id}", gw.handleSoulUsage)
		r.Get("/usage/account", gw.handleAccountUsage)
		r.Post("/pricing/refresh", gw.handleRefreshPricing)

		// Smart Routing
		r.Get("/routing/categories", gw.handleListCategories)
		r.Get("/routing/assignments", gw.handleGetAssignments)
		r.Get("/routing/suggestions", gw.handleRoutingSuggestions)
		r.Get("/routing/model-rankings", gw.handleModelRankings)
		r.Post("/routing/assign", gw.handleAssignModel)
		r.Delete("/routing/assign", gw.handleUnassignModel)
		r.Post("/routing/classify", gw.handleClassifyQuery)
		r.Post("/routing/score", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Query string `json:"query"`; HasTools bool `json:"has_tools"`; HasImages bool `json:"has_images"`; HistoryLen int `json:"history_len"` }
			json.NewDecoder(r.Body).Decode(&req)
			dims := providers.ScoreRequest(req.Query, req.HasTools, req.HasImages, req.HistoryLen)
			tier := providers.SelectTier(dims, "")
			writeJSON(w, 200, map[string]any{"dimensions": dims, "tier": tier})
		})
		// Tool execution metrics
		r.Get("/tools/metrics", func(w http.ResponseWriter, r *http.Request) {
			if gw.toolReg != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Write(gw.toolReg.Metrics().JSON())
			} else {
				writeJSON(w, 200, map[string]any{"tools": []any{}})
			}
		})
		// ─── Voice provider catalog + CRUD (plug-and-play) ─────────────
		//
		// The legacy hardcoded-switch shape of these routes shipped in
		// the early voice spike — covered exactly four providers and
		// required a gateway release to add a fifth. Replaced with
		// catalog + DB-driven handlers that mirror the LLM provider
		// admin surface. See voice_provider_handlers.go.
		r.Get("/voice/catalog",               gw.handleVoiceCatalog)
		r.Get("/voice/providers",             gw.handleVoiceProvidersList)
		r.Post("/voice/providers",            gw.handleVoiceProvidersCreate)
		r.Put("/voice/providers/{id}",        gw.handleVoiceProvidersUpdate)
		r.Delete("/voice/providers/{id}",     gw.handleVoiceProvidersDelete)
		r.Post("/voice/providers/{id}/default", gw.handleVoiceProvidersSetDefault)
		r.Post("/voice/providers/{id}/test",    gw.handleVoiceProvidersTest)

		// ─── Media generation provider catalog + CRUD ────────────────────
		r.Get("/media/catalog",                   gw.handleMediaCatalog)
		r.Get("/media/providers",                 gw.handleMediaProvidersList)
		r.Post("/media/providers",                gw.handleMediaProvidersCreate)
		r.Put("/media/providers/{id}",            gw.handleMediaProvidersUpdate)
		r.Delete("/media/providers/{id}",         gw.handleMediaProvidersDelete)
		r.Post("/media/providers/{id}/default",   gw.handleMediaProvidersSetDefault)
		r.Post("/media/providers/{id}/test",      gw.handleMediaProvidersTest)

		// === Supervisor API ===
		r.Get("/supervisor/status", func(w http.ResponseWriter, r *http.Request) {
			if gw.supervisorBus == nil { writeJSON(w, 200, map[string]any{"status": "not_initialized"}); return }
			writeJSON(w, 200, gw.supervisorBus.Stats())
		})
		r.Get("/supervisor/health", func(w http.ResponseWriter, r *http.Request) {
			if gw.supervisor == nil { writeJSON(w, 200, map[string]any{"agents": []any{}}); return }
			writeJSON(w, 200, map[string]any{"agents": gw.supervisor.AgentHealthList()})
		})
		r.Get("/supervisor/audit-log", func(w http.ResponseWriter, r *http.Request) {
			if gw.supervisorBus == nil { writeJSON(w, 200, map[string]any{"messages": []any{}}); return }
			limit := 50
			writeJSON(w, 200, map[string]any{"messages": gw.supervisorBus.AuditLog(limit)})
		})
		r.Get("/supervisor/escalations", func(w http.ResponseWriter, r *http.Request) {
			if gw.supervisorBus == nil { writeJSON(w, 200, map[string]any{"escalations": []any{}}); return }
			writeJSON(w, 200, map[string]any{"escalations": gw.supervisorBus.PendingEscalations()})
		})
		r.Post("/supervisor/escalations/{id}/approve", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if gw.supervisor == nil { writeJSON(w, 503, map[string]string{"error": "supervisor not initialized"}); return }
			var req struct{ Reason string `json:"reason"` }
			json.NewDecoder(r.Body).Decode(&req)
			gw.supervisor.ResolveEscalation(r.Context(), id, true, req.Reason)
			writeJSON(w, 200, map[string]string{"status": "approved"})
		})
		r.Post("/supervisor/escalations/{id}/reject", func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, "id")
			if gw.supervisor == nil { writeJSON(w, 503, map[string]string{"error": "supervisor not initialized"}); return }
			var req struct{ Reason string `json:"reason"` }
			json.NewDecoder(r.Body).Decode(&req)
			gw.supervisor.ResolveEscalation(r.Context(), id, false, req.Reason)
			writeJSON(w, 200, map[string]string{"status": "rejected"})
		})
		r.Post("/supervisor/agents/{id}/unsuspend", func(w http.ResponseWriter, r *http.Request) {
			agentID := chi.URLParam(r, "id")
			if gw.supervisor == nil { writeJSON(w, 503, map[string]string{"error": "supervisor not initialized"}); return }
			gw.supervisor.Unsuspend(agentID)
			writeJSON(w, 200, map[string]string{"status": "unsuspended", "agent_id": agentID})
		})
		r.Get("/supervisor/fixes", func(w http.ResponseWriter, r *http.Request) {
			if gw.supervisor == nil { writeJSON(w, 200, map[string]any{"fixes": []any{}}); return }
			catalog := gw.supervisor.Catalog()
			writeJSON(w, 200, map[string]any{"available": catalog.ListFixes(), "history": catalog.History(20)})
		})



		// === Code Pipeline API ===
		r.Post("/pipeline/propose", func(w http.ResponseWriter, r *http.Request) {
			if gw.codePipeline == nil { writeJSON(w, 503, map[string]string{"error": "pipeline not initialized"}); return }
			var req struct {
				Description string                `json:"description"`
				Files       []systempkg.FileChange `json:"files"`
				Risk        string                `json:"risk"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			change, err := gw.codePipeline.Propose(r.Context(), req.Description, req.Files, req.Risk, "api")
			if err != nil { writeJSON(w, 400, map[string]string{"error": err.Error()}); return }
			writeJSON(w, 200, change)
		})
		r.Post("/pipeline/validate/{id}", func(w http.ResponseWriter, r *http.Request) {
			if gw.codePipeline == nil { writeJSON(w, 503, map[string]string{"error": "pipeline not initialized"}); return }
			id := chi.URLParam(r, "id")
			if err := gw.codePipeline.Validate(r.Context(), id); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error(), "change": id})
				return
			}
			writeJSON(w, 200, gw.codePipeline.Get(id))
		})
		r.Post("/pipeline/apply/{id}", func(w http.ResponseWriter, r *http.Request) {
			if gw.codePipeline == nil { writeJSON(w, 503, map[string]string{"error": "pipeline not initialized"}); return }
			id := chi.URLParam(r, "id")
			if err := gw.codePipeline.Apply(r.Context(), id); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error(), "change": id})
				return
			}
			writeJSON(w, 200, gw.codePipeline.Get(id))
		})
		r.Get("/pipeline/changes", func(w http.ResponseWriter, r *http.Request) {
			if gw.codePipeline == nil { writeJSON(w, 200, map[string]any{"changes": []any{}}); return }
			writeJSON(w, 200, map[string]any{"changes": gw.codePipeline.List()})
		})
		r.Get("/pipeline/pending", func(w http.ResponseWriter, r *http.Request) {
			if gw.codePipeline == nil { writeJSON(w, 200, map[string]any{"pending": []any{}}); return }
			writeJSON(w, 200, map[string]any{"pending": gw.codePipeline.Pending()})
		})
		// === Company Memory API ===
		r.Get("/memory/company", func(w http.ResponseWriter, r *http.Request) {
			if gw.memStore == nil { writeJSON(w, 200, map[string]any{"memories": []any{}}); return }
			mems, _ := gw.memStore.SearchByType(r.Context(), "00000000-0000-0000-0000-000000000001", "company", 50)
			writeJSON(w, 200, map[string]any{"memories": mems})
		})
		r.Post("/memory/company", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Content string `json:"content"`; Source string `json:"source"` }
			json.NewDecoder(r.Body).Decode(&req)
			if req.Content == "" { writeJSON(w, 400, map[string]string{"error": "content required"}); return }
			if gw.agentLoop.HierarchyMem == nil { writeJSON(w, 503, map[string]string{"error": "memory not initialized"}); return }
			id, err := gw.agentLoop.HierarchyMem.SaveCompany(r.Context(), req.Content, req.Source)
			if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
			writeJSON(w, 200, map[string]string{"id": id, "status": "saved"})
		})
		r.Post("/memory/prime", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Content string `json:"content"`; Source string `json:"source"` }
			json.NewDecoder(r.Body).Decode(&req)
			if req.Content == "" { writeJSON(w, 400, map[string]string{"error": "content required"}); return }
			if gw.agentLoop.HierarchyMem == nil { writeJSON(w, 503, map[string]string{"error": "memory not initialized"}); return }
			id, err := gw.agentLoop.HierarchyMem.SavePrime(r.Context(), req.Content, req.Source)
			if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
			writeJSON(w, 200, map[string]string{"id": id, "status": "saved"})
		})
		r.Post("/memory/search", gw.handleMemorySearch)
		// GET variant: q= and optional agent_id= / max_results= — for the command palette
		r.Get("/memory/search", gw.handleMemorySearchGET)
		r.Post("/memory/save", gw.handleMemorySave)
		r.Get("/memory/scopes", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, map[string]any{"scopes": []string{"company", "team", "agent", "task", "session", "prime"}})
		})
		r.Get("/teams", gw.handleListTeams)
		r.Post("/teams", gw.handleCreateTeam)
		r.Get("/teams/{id}/members", gw.handleTeamMembers)
		r.Get("/plugins", gw.handleListPlugins)
		r.Post("/plugins/install", gw.handleInstallPlugin)
		r.Delete("/plugins/{name}", gw.handleRemovePlugin)

		// tenant-scoped Wasm plugin registry. Distinct from
		// the legacy `/v1/plugins` tree above which serves the in-memory
		// PluginManager — those two systems are kept apart pending a
		// Phase 6 consolidation.
		//
		// mutations (POST/DELETE) carry the quota — an
		// AI-generated pipeline that keeps re-uploading a plugin can
		// burn through connection slots. The GET listing is cheap;
		// leave it unquota'd so dashboards stay responsive.
		r.With(gw.TenantQuotaMiddleware).Post("/wasm-plugins", gw.handleUploadWasmPlugin)
		r.Get("/wasm-plugins", gw.handleListWasmPlugins)
		r.With(gw.TenantQuotaMiddleware).Delete("/wasm-plugins/{name}", gw.handleRevokeWasmPlugin)
		r.Get("/agents/{id}/channels", gw.handleListAgentChannels)
		r.Post("/agents/{id}/channels", gw.handleBindAgentChannel)
		r.Delete("/agents/{id}/channels/{bindingId}", gw.handleUnbindAgentChannel)

		// Inbound automation config + rules
		r.Route("/agents/{id}/inbound-config", func(r chi.Router) {
			r.Get("/", gw.handleGetInboundConfig)
			r.Put("/", gw.handlePutInboundConfig)
		})
		r.Route("/agents/{id}/inbound-rules", func(r chi.Router) {
			r.Get("/", gw.handleListInboundRules)
			r.Post("/", gw.handleCreateInboundRule)
			r.Put("/{ruleId}", gw.handleUpdateInboundRule)
			r.Delete("/{ruleId}", gw.handleDeleteInboundRule)
			r.Post("/{ruleId}/confirm", gw.handleConfirmInboundRule)
			r.Post("/{ruleId}/discard", gw.handleDiscardInboundRule)
		})

		// Draft replies queue
		r.Get("/drafts", gw.handleListDrafts)
		r.Get("/drafts/{id}", gw.handleGetDraft)
		r.Post("/drafts/{id}/send", gw.handleSendDraft)
		r.Post("/drafts/{id}/discard", gw.handleDiscardDraft)
		r.Post("/drafts/{id}/edit", gw.handleEditDraft)

		// === App Platform ===
		r.Route("/apps", func(r chi.Router) {
			r.Get("/", gw.handleListApps)
			r.Post("/", gw.handleInstallApp)
			r.Get("/{id}", gw.handleGetApp)
			r.Patch("/{id}", gw.handlePatchApp)
			r.Delete("/{id}", gw.handleUninstallApp)
			r.Post("/{id}/reload", gw.handleReloadApp)
			r.Post("/{slug}/tools/{name}", gw.handleRunAppTool)
		})
		// === Council API ===
		r.Post("/council", func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Query    string   `json:"query"`
				Members  []string `json:"members"`
				Chairman string   `json:"chairman"`
				Depth    string   `json:"depth"` // quick, balanced, deep, max
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.Query == "" { writeJSON(w, 400, map[string]string{"error": "query required"}); return }

			cfg := council.DefaultConfig()
			if len(req.Members) > 0 { cfg.Members = req.Members }
			if req.Chairman != "" { cfg.Chairman = req.Chairman }

			// Check depth dial
			depth := council.Depth(req.Depth)
			if depth == "" { depth = council.DepthDeep }
			depthCfg := council.GetDepthConfig(depth)
			if !depthCfg.CouncilEnabled {
				writeJSON(w, 400, map[string]string{"error": "council not available at depth " + string(depth)})
				return
			}

			provider := gw.providerReg.Default()
			if provider == nil { writeJSON(w, 503, map[string]string{"error": "no provider"}); return }

			c := council.New(provider, cfg)
			// Route each council member to the provider that actually
			// owns its model — without this every member goes to the
			// single default provider and Bedrock-only members 404.
			c.Resolver = gw.providerReg.ProviderForModel
			result, err := c.Run(r.Context(), req.Query)
			if err != nil { writeJSON(w, 500, map[string]string{"error": err.Error()}); return }
			writeJSON(w, 200, result)
		})
		r.Get("/council/config", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, 200, map[string]any{
				"default": council.DefaultConfig(),
				"depths":  council.DepthConfigs,
			})
		})
		r.Get("/models/excluded", func(w http.ResponseWriter, r *http.Request) {
			if gw.agentLoop.SmartRouter != nil {
				writeJSON(w, 200, map[string]any{"excluded": gw.agentLoop.SmartRouter.Exclusion().List()})
			} else { writeJSON(w, 200, map[string]any{"excluded": []string{}}) }
		})
		r.Post("/models/exclude", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Model string `json:"model"` }
			json.NewDecoder(r.Body).Decode(&req)
			if gw.agentLoop.SmartRouter != nil { gw.agentLoop.SmartRouter.Exclusion().Add(req.Model) }
			writeJSON(w, 200, map[string]string{"status": "excluded", "model": req.Model})
		})
		r.Delete("/models/exclude", func(w http.ResponseWriter, r *http.Request) {
			var req struct{ Model string `json:"model"` }
			json.NewDecoder(r.Body).Decode(&req)
			if gw.agentLoop.SmartRouter != nil { gw.agentLoop.SmartRouter.Exclusion().Remove(req.Model) }
			writeJSON(w, 200, map[string]string{"status": "removed", "model": req.Model})
		})
		r.Get("/routing/decisions", gw.handleRecentDecisions)
		r.Post("/routing/correct", gw.handleCorrectDecision)
		r.Get("/models", gw.handleListModels)

		// OpenAI-compatible embeddings endpoint
		r.Post("/embeddings", gw.handleEmbeddings)

		// /btw side-question — quick answer without polluting session
		r.Post("/btw", gw.handleBTW)

		// Tools
		r.Get("/tools/builtin", gw.handleListBuiltinTools)
		r.Get("/tools/custom", gw.handleListCustomTools)
		r.Post("/tools/custom", gw.handleCreateCustomTool)
		r.Delete("/tools/custom/{id}", gw.handleDeleteCustomTool)

		// Skills
		r.Get("/skills", gw.handleListSkills)
		r.Delete("/skills/{id}", gw.handleDeleteSkill)
		r.Patch("/skills/{id}", gw.handlePatchSkill)

		// Skills Marketplace
		r.Get("/marketplace/skills", gw.handleMarketplaceSkills)
		r.Get("/marketplace/skills/{slug}", gw.handleGetSkill)
		r.Post("/marketplace/skills", gw.handlePublishSkill)
		r.Post("/marketplace/skills/{slug}/install", gw.handleInstallSkill)
		r.Post("/marketplace/skills/{slug}/uninstall", gw.handleUninstallSkill)
		r.Post("/marketplace/skills/{slug}/rate", gw.handleRateSkill)
		r.Get("/agents/{id}/skills", gw.handleAgentSkills)

		// Notifications
		r.Get("/notifications", gw.handleListNotifications)
		r.Post("/notifications/{id}/read", gw.handleMarkNotificationRead)
		r.Post("/notifications/read-all", gw.handleMarkAllNotificationsRead)
		r.Get("/notifications/unread-count", func(w http.ResponseWriter, r *http.Request) {
			if gw.notifStore == nil { writeJSON(w, 200, map[string]any{"count": 0}); return }
			_, unread, _ := gw.notifStore.List(r.Context(), 1)
			writeJSON(w, 200, map[string]any{"count": unread})
		})

		// Live activity
		r.Get("/activity", func(w http.ResponseWriter, r *http.Request) {
			// Return recent supervisor messages + notifications as activity feed
			activity := []map[string]any{}
			if gw.supervisorBus != nil {
				for _, msg := range gw.supervisorBus.AuditLog(20) {
					activity = append(activity, map[string]any{
						"type": "supervisor", "intent": msg.Intent, "from": msg.From, "to": msg.To,
						"content": msg.Content, "timestamp": msg.Timestamp,
					})
				}
			}
			if gw.notifStore != nil {
				notifs, _, _ := gw.notifStore.List(r.Context(), 20)
				for _, n := range notifs {
					activity = append(activity, map[string]any{
						"type": "notification", "title": n.Title, "highlight": n.Highlight,
						"source": n.Source, "read": n.Read, "timestamp": n.CreatedAt,
					})
				}
			}
			writeJSON(w, 200, map[string]any{"activity": activity})
		})

		// MCP
		r.Get("/mcp/servers", gw.handleListMCPServers)
		r.Post("/mcp/servers", gw.handleConnectMCPServer)
		r.Get("/mcp/servers/{name}", gw.handleGetMCPServer)
		r.Post("/mcp/servers/{name}/test", gw.handleTestMCPServer)
		r.Get("/mcp/servers/{name}/tools", gw.handleGetMCPServerTools)
		r.Delete("/mcp/servers/{name}", gw.handleDisconnectMCPServer)
		r.Get("/mcp/tools", gw.handleListMCPTools)

		// Tasks
		r.Get("/tasks", gw.handleListTasks)
		r.Post("/tasks", gw.handleCreateTask)
		r.Get("/tasks/{id}", gw.handleGetTask)
		r.Put("/tasks/{id}/status", gw.handleUpdateTaskStatus)
		r.Post("/tasks/{id}/cancel", gw.handleCancelTask)
		r.Post("/tasks/{id}/pause", gw.handleTaskPause)
		r.Post("/tasks/{id}/resume", gw.handleTaskResume)
		r.Post("/tasks/{id}/message", gw.handleTaskMessage)
		r.Get("/tasks/{id}/comments", gw.handleListTaskComments)
		r.Post("/tasks/{id}/comments", gw.handleAddTaskComment)
		r.Get("/tasks/{id}/files", gw.handleListTaskFiles)
		r.Get("/tasks/{id}/events", gw.handleListTaskEvents)

		// Agent Messages
		r.Get("/agents/{id}/messages", gw.handleGetAgentMessages)
		r.Post("/agents/{id}/messages", gw.handleSendAgentMessage)

		// Org Chart
		r.Get("/org-chart", gw.handleGetOrgChart)

		// Budget tracking (P4.1)
		r.Get("/budgets", gw.handleGetBudgets)
		r.Put("/agents/{id}/budget", gw.handleSetBudget)

		// Heartbeat
		r.Get("/agents/{id}/heartbeat", gw.handleGetHeartbeat)
		r.Put("/agents/{id}/heartbeat", gw.handleUpsertHeartbeat)

		// QOROS — proactive agent mode
		r.Post("/agents/{id}/qoros/start", gw.handleStartQoros)
		r.Post("/agents/{id}/qoros/stop", gw.handleStopQoros)
		r.Get("/agents/{id}/qoros/status", gw.handleQorosStatus)

		// Dreaming — memory consolidation config
		r.Get("/agents/{id}/dreaming", gw.handleGetDreaming)
		r.Put("/agents/{id}/dreaming", gw.handleUpdateDreaming)
		r.Post("/agents/{id}/dreaming/trigger", gw.handleTriggerDream)

		// Cron jobs
		r.Get("/cron-jobs", gw.handleListCronJobs)
		r.Post("/cron-jobs", gw.handleCreateCronJob)
		r.Post("/cron-jobs/{id}/pause", gw.handlePauseCronJob)
		r.Post("/cron-jobs/{id}/resume", gw.handleResumeCronJob)
		r.Post("/cron-jobs/{id}/toggle", gw.handleToggleCronJob)
		r.Delete("/cron-jobs/{id}", gw.handleDeleteCronJob)

		// Contacts (lightweight CRM)
		r.Get("/contacts", gw.handleListContacts)
		r.Post("/contacts", gw.handleCreateContact)
		r.Get("/contacts/{id}", gw.handleGetContact)
		r.Patch("/contacts/{id}", gw.handlePatchContact)
		r.Get("/contacts/{id}/prefs/{agentId}", gw.handleGetContactPrefs)
		r.Put("/contacts/{id}/prefs/{agentId}", gw.handlePutContactPrefs)

		// Channels
		r.Get("/channels", gw.handleListChannels)
		r.Post("/channels", gw.handleCreateChannel)
		r.Get("/channels/{id}", gw.handleGetChannel)
		r.Put("/channels/{id}", gw.handleUpdateChannel)
		r.Delete("/channels/{id}", gw.handleDeleteChannel)
		r.Post("/channels/{id}/start", gw.handleStartChannel)
		r.Post("/channels/{id}/stop", gw.handleStopChannel)
		r.Post("/channels/{id}/test", gw.handleTestChannel)
		// WhatsApp bridge endpoints
		r.Get("/channels/{id}/whatsapp/qr", gw.handleWhatsAppQRStream)
		r.Get("/channels/{id}/whatsapp/pending", gw.handleWhatsAppListPending)
		r.Post("/channels/{id}/whatsapp/pending/{pendingId}/approve", gw.handleWhatsAppApproveSender)
		r.Post("/channels/{id}/whatsapp/pending/{pendingId}/deny", gw.handleWhatsAppDenySender)

		// Rooms
		r.Get("/rooms", gw.handleListRooms)
		r.Post("/rooms", gw.handleCreateRoom)
		r.Get("/rooms/{id}", gw.handleGetRoom)
		r.Delete("/rooms/{id}", gw.handleDeleteRoom)
		r.Post("/rooms/{id}/members", gw.handleAddRoomMember)
		r.Delete("/rooms/{id}/members/{agent_id}", gw.handleRemoveRoomMember)
		r.Get("/rooms/{id}/messages", gw.handleGetRoomMessages)
		r.Post("/rooms/{id}/messages", gw.handlePostRoomMessage)
		// Room v2 — decisions, minutes, typing, tasks
		r.Get("/rooms/{id}/decisions", gw.handleGetRoomDecisions)
		r.Post("/rooms/{id}/decisions", gw.handlePostRoomDecision)
		r.Get("/rooms/{id}/minutes", gw.handleGetRoomMinutes)
		r.Post("/rooms/{id}/minutes/generate", gw.handleGenerateRoomMinutes)
		r.Post("/rooms/{id}/typing", gw.handleRoomTyping)
		r.Get("/rooms/{id}/tasks", gw.handleGetRoomTasks)
		r.Post("/rooms/{id}/tasks", gw.handleCreateRoomTask)
		r.Put("/rooms/{id}/tasks/{task_id}", gw.handleUpdateRoomTask)
		r.Get("/rooms/{id}/org", gw.handleGetRoomOrg)

		// Goals (Pattern 11)
		r.Get("/goals", gw.handleListGoals)
		r.Post("/goals", gw.handleCreateGoal)

		// Approvals (Pattern 13)
		r.Get("/user/me", gw.handleMe)
		r.Get("/user/preferences", gw.handleGetPreferences)
		r.Post("/user/preferences", gw.handleSavePreferences)
		r.Patch("/user/profile", gw.handlePatchProfile)
		r.Get("/user/api-keys", gw.handleListAPIKeys)
		r.Delete("/user/api-keys/{id}", gw.handleRevokeAPIKey)
		r.Get("/user/sessions", gw.handleListUserSessions)
		r.Delete("/user/sessions/{id}", gw.handleRevokeUserSession)
		r.Get("/approvals", gw.handleListApprovals)
		r.Post("/approvals/{id}/decide", gw.handleDecideApproval)

		// Outbound approval queue
		r.Get("/outbound/pending", gw.handleOutboundPending)
		r.Post("/outbound/{id}/approve", gw.handleOutboundApprove)
		r.Post("/outbound/{id}/reject", gw.handleOutboundReject)

		// Metrics (Pattern 19)
		r.Get("/metrics/{id}", gw.handleGetMetrics)

		// Workflows
		r.Get("/workflows", gw.handleListWorkflows)
		r.Post("/workflows", gw.handleCreateWorkflow)
		r.Get("/workflows/{id}", gw.handleGetWorkflow)
		r.Put("/workflows/{id}", gw.handleUpdateWorkflow)
		r.Delete("/workflows/{id}", gw.handleDeleteWorkflow)
		r.Post("/workflows/{id}/run", gw.handleRunWorkflow)
		r.Post("/workflows/{id}/toggle", gw.handleToggleWorkflow)
		r.Get("/workflows/{id}/runs", gw.handleListWorkflowRuns)

		// Research
		r.Post("/research/start", gw.handleResearchStart)
		r.Get("/research/{id}", gw.handleResearchGet)

		// Voice (TTS + STT)
		r.Post("/audio/speech", gw.handleTTS)
		r.Post("/audio/transcribe", gw.handleSTT)
		r.Get("/audio/providers", gw.handleVoiceProviders)

		// Training data export
		r.Get("/training/export/{agent_id}", gw.handleTrainingExport)

		// Crystallized skills (OpenSpace self-evolving)
		r.Get("/skills/crystallized/{agent_id}", gw.handleListCrystallizedSkills)
		r.Post("/skills/crystallized/{id}/promote", gw.handlePromoteSkill)

		// Connectors
		r.Get("/connectors", gw.handleListConnectors)
		r.Post("/connectors/{id}/test", gw.handleTestConnector)
		r.Post("/connectors/{id}/execute", gw.handleExecuteConnector)

		// Connector knowledge + OAuth + connections
		r.Get("/connections", gw.handleListConnections)
		r.Post("/connections/{platform_id}", gw.handleSaveConnection)
		r.Delete("/connections/{platform_id}", gw.handleDeleteConnection)

		// GitHub proxy — used by the /code page GitHub panel.
		// All requests are forwarded to api.github.com using the vault token.
		r.Get("/github/{owner}/{repo}/info", gw.handleGitHubRepoInfo)
		r.Get("/github/{owner}/{repo}/issues", gw.handleGitHubListIssues)
		r.Get("/github/{owner}/{repo}/pulls", gw.handleGitHubListPulls)
		r.Post("/github/{owner}/{repo}/pulls/{pr}/merge", gw.handleGitHubMergePR)
		r.Get("/github/{owner}/{repo}/pulls/{prNum}/checks", gw.handleGitHubPRChecks)
		r.Post("/github/{owner}/{repo}/issues/{number}/close", gw.handleGitHubCloseIssue)
		// GitHub autonomous task queue (in-memory GitHubTaskQueue)
		r.Get("/github/tasks", gw.handleListGitHubTasks)
		r.Get("/github/tasks/{id}", gw.handleGetGitHubTask)
		r.Post("/github/tasks/{id}/advance", gw.handleAdvanceGitHubTask)
		r.Post("/github/tasks/{id}/block", gw.handleBlockGitHubTask)
		r.Get("/connectors/platforms", gw.handleListPlatforms)
		r.Get("/connectors/platforms/{id}/actions", gw.handleListPlatformActions)
		r.Post("/connectors/execute-action", gw.handleExecuteAction)
		r.Get("/oauth/{provider}/authorize", gw.handleOAuthAuthorize)
		r.Get("/oauth/{provider}/callback", gw.handleOAuthCallback)
		// Per-tenant OAuth app credentials — operators register
		// client_id+secret here, overriding the env defaults on a
		// per-tenant basis.
		r.Get("/oauth/apps", gw.handleListOAuthApps)
		r.Post("/oauth/apps/{provider}", gw.handleSetOAuthApp)
		r.Delete("/oauth/apps/{provider}", gw.handleDeleteOAuthApp)

		// Social Media Publishing
		r.Get("/social/posts", gw.handleListSocialPosts)
		r.Post("/social/posts", gw.handleCreateSocialPost)
		r.Get("/social/posts/{id}", gw.handleGetSocialPost)
		r.Delete("/social/posts/{id}", gw.handleDeleteSocialPost)
		r.Post("/social/posts/{id}/publish", gw.handlePublishSocialPost)
		r.Get("/social/integrations", gw.handleListSocialIntegrations)
		r.Post("/social/integrations", gw.handleSaveSocialIntegration)
		r.Delete("/social/integrations/{id}", gw.handleDeleteSocialIntegration)
		r.Get("/social/autoposts", gw.handleListSocialAutoPosts)
		r.Post("/social/autoposts", gw.handleCreateSocialAutoPost)
		r.Delete("/social/autoposts/{id}", gw.handleDeleteSocialAutoPost)
		r.Get("/social/calendar", gw.handleSocialCalendar)

		// Dead-letter queue (FU-021)
		r.Get("/admin/dead-letters", gw.handleListDeadLetters)
		r.Post("/admin/reset/{target}", gw.handleAdminReset)
		r.Post("/admin/factory-reset", gw.handleAdminFactoryReset)
		r.Get("/admin/update/check", gw.handleAdminUpdateCheck)
		r.Post("/admin/update/install", gw.handleAdminUpdateInstall)

		// Service accounts
		r.Get("/service-accounts", gw.handleListServiceAccounts)
		r.Post("/service-accounts", gw.handleCreateServiceAccount)
		r.Delete("/service-accounts/{id}", gw.handleRevokeServiceAccount)

		// Audit + Billing
		r.Post("/feedback", gw.handleMessageFeedback)
		r.Get("/audit", gw.handleAuditLog)
		r.Get("/billing/costs", gw.handleBillingCosts)

		// Mail
		r.Get("/mail/identities", gw.handleListMailIdentities)
		r.Post("/mail/identities", gw.handleCreateMailIdentity)
		r.Put("/mail/identities/{id}", gw.handleUpdateMailIdentity)
		r.Get("/mail/aliases", gw.handleListMailAliases)
		r.Post("/mail/aliases", gw.handleCreateMailAlias)
		r.Delete("/mail/aliases/{id}", gw.handleDeleteMailAlias)
		r.Get("/mail/inbox", gw.handleMailInbox)
		r.Get("/mail/sent", gw.handleMailSent)
		r.Get("/mail/{id}", gw.handleGetMail)
		r.Get("/mail/thread/{thread_id}", gw.handleGetMailThread)
		r.Post("/mail/send", gw.handleSendMail)
		r.Put("/mail/{id}/read", gw.handleMailRead)
		r.Put("/mail/{id}/star", gw.handleMailStar)
		r.Get("/approvals/mail", gw.handleListMailApprovals)
		r.Post("/approvals/mail/{id}/approve", gw.handleApproveMailFunc)
		r.Post("/approvals/mail/{id}/reject", gw.handleRejectMailFunc)

		// Drive
		r.Get("/drive/files", gw.handleListDriveFiles)
		r.Post("/drive/upload", gw.handleUploadFile)
		r.Get("/drive/files/{id}/download", gw.handleDownloadFile)
		r.Post("/drive/folders", gw.handleCreateFolder)
		r.Put("/drive/files/{id}/share", gw.handleShareFile)
		r.Delete("/drive/files/{id}", gw.handleDeleteDriveFile)
		r.Get("/drive/quota", gw.handleDriveQuota)
		r.Post("/drive/files/{id}/enrich", gw.handleEnrichFile)

		// Sandbox
		r.Route("/sandbox", func(r chi.Router) {
			r.Post("/run", gw.handleSandboxRun)
			r.Get("/runs", gw.handleListSandboxRuns)
			r.Get("/runs/{id}", gw.handleGetSandboxRun)
			r.Get("/artifacts", gw.handleListArtifacts)
			r.Get("/apps", gw.handleListSandboxApps)
			r.Delete("/apps/{id}", gw.handleStopSandboxApp)
		})

		// Calendar
		r.Get("/calendar/events", gw.handleListCalendarEvents)
		r.Post("/calendar/events", gw.handleCreateCalendarEvent)
		r.Put("/calendar/events/{id}", gw.handleUpdateCalendarEvent)
		r.Delete("/calendar/events/{id}", gw.handleDeleteCalendarEvent)

		// Pairing
		r.Get("/pairing/pending", gw.handleListPairingRequests)
		r.Post("/pairing/approve", gw.handleApprovePairing)
		r.Get("/pairing/devices", gw.handleListPairedDevices)

		// Tasks full update
		r.Put("/tasks/{id}", gw.handleUpdateTask)

		// System + Voice
		r.Get("/changelog", gw.handleGetChangelog)
		r.Get("/system/specs", gw.handleSystemSpecs)
		r.Get("/system/info", gw.handleSystemSpecs)
		r.Post("/system/install", gw.handleSystemInstall)
		r.Get("/system/install/status", gw.handleInstallStatus)
		r.Get("/network/status", gw.handleNetworkStatus)
		r.Post("/network/tailscale", gw.handleNetworkTailscale)
		r.Get("/voice/config", gw.handleGetVoiceConfig)
		r.Put("/voice/config", gw.handlePutVoiceConfig)

		// Templates / Marketplace
		r.Get("/templates", gw.handleListTemplates)
		r.Post("/templates/install", gw.handleInstallTemplate)
		r.Get("/templates/installed", gw.handleListInstalled)
		r.Get("/templates/{id}/dashboard", gw.handleGetDashboard)
		r.Post("/dashboards", gw.handleSaveDashboard)
		r.Get("/dashboards/{id}", gw.handleGetDashboardByID)
		r.Post("/templates/self-build", gw.handleSelfBuild)
		r.Get("/templates/{id}/export", gw.handleExportTemplate)
		r.Post("/templates/import", gw.handleImportTemplate)


		// MCP servers
		r.Get("/integrations/mcp", gw.handleListMCPDB)
		r.Post("/integrations/mcp", gw.handleAddMCPDB)
		r.Post("/integrations/mcp/{id}/install", gw.handleInstallMCPDB)
		r.Delete("/integrations/mcp/{id}", gw.handleDeleteMCPDB)

		// Knowledge Graph visualization API
		r.Get("/graph", gw.handleGraphData)
		r.Get("/graph/{nodeId}", gw.handleGraphNeighborhood)
		r.Get("/graph/{nodeId}/relevance", gw.handleGraphRelevance)
		r.Get("/graph/god-nodes", gw.handleGodNodes)
		r.Get("/graph/clusters", gw.handleGraphClusters)
		r.Get("/graph/analysis", gw.handleGraphAnalysis)

		// PTY terminal sessions (browser ↔ shell over WebSocket)
		r.Get("/terminal/sessions", gw.handleListTerminalSessions)
		r.Post("/terminal/sessions", gw.handleCreateTerminalSession)
		r.Delete("/terminal/sessions/{id}", gw.handleDeleteTerminalSession)
		r.Get("/terminal/sessions/{id}/ws", gw.handleTerminalWS)

		// Work goals
		r.Get("/work-goals", gw.handleListWorkGoals)
		r.Get("/work-goals/tree", gw.handleGetWorkGoalTree)
		r.Post("/work-goals", gw.handleCreateWorkGoal)
		r.Put("/work-goals/{id}", gw.handleUpdateWorkGoal)
		r.Delete("/work-goals/{id}", gw.handleDeleteWorkGoal)

		// Tickets
		r.Get("/tickets", gw.handleListTickets)
		r.Post("/tickets", gw.handleCreateTicket)
		r.Put("/tickets/{id}", gw.handleUpdateTicket)
		r.Delete("/tickets/{id}", gw.handleDeleteTicket)
		r.Post("/tickets/{id}/assign", gw.handleAssignTicket)
		r.Get("/tickets/{id}/comments", gw.handleListTicketComments)
		r.Post("/tickets/{id}/comments", gw.handleAddTicketComment)
		r.Get("/tickets/{id}/files", gw.handleListTicketFiles)

		// Project Briefs (inception flow)
		r.Get("/project-briefs", gw.handleListProjectBriefs)
		r.Post("/project-briefs", gw.handleCreateProjectBrief)
		r.Get("/project-briefs/{id}", gw.handleGetProjectBrief)
		r.Put("/project-briefs/{id}", gw.handleUpdateProjectBrief)
		r.Post("/project-briefs/{id}/propose", gw.handleProposeTeam)
		r.Post("/project-briefs/{id}/approve", gw.handleApproveTeam)
		r.Get("/project-briefs/{id}/team", gw.handleGetBriefTeam)

		// GitHub webhook
		r.Post("/webhooks/github", gw.handleGitHubWebhook)

		// Dashboard tiles — pinned data tiles driven by connector snapshots.
		r.Route("/dashboard", func(r chi.Router) {
			r.Get("/tiles", gw.listDashboardTiles)
			r.Post("/tiles", gw.createDashboardTile)
			r.Delete("/tiles/{id}", gw.deleteDashboardTile)
		})

		// Multi-agent daemon: external agent registry, task queue, plan approval.
		// Agents (kiro-cli, claude-code) register here and receive tasks via SSE push.
		r.Route("/daemon", func(d chi.Router) {
			// Agent registry
			d.Get("/agents", gw.handleDaemonListAgents)
			d.Post("/agents/register", gw.handleDaemonRegisterAgent)
			d.Delete("/agents/{id}", gw.handleDaemonUnregisterAgent)
			d.Post("/agents/{id}/heartbeat", gw.handleDaemonAgentHeartbeat)
			// SSE stream — connect here to receive task_assigned and all daemon events
			d.Get("/stream", gw.handleDaemonStream)
			// Task queue
			d.Get("/tasks", gw.handleDaemonListTasks)
			d.Post("/tasks", gw.handleDaemonCreateTask)
			d.Get("/tasks/{id}", gw.handleDaemonGetTask)
			d.Post("/tasks/{id}/assign", gw.handleDaemonAssignTask)
			d.Post("/tasks/{id}/progress", gw.handleDaemonTaskProgress)
			d.Post("/tasks/{id}/complete", gw.handleDaemonTaskComplete)
			d.Post("/tasks/{id}/fail", gw.handleDaemonTaskFail)
			// Plan approval
			d.Get("/plans", gw.handleDaemonListPlans)
			d.Post("/plans", gw.handleDaemonProposePlan)
			d.Get("/plans/{id}", gw.handleDaemonGetPlan)
			d.Post("/plans/{id}/approve", gw.handleDaemonApprovePlan)
			d.Post("/plans/{id}/reject", gw.handleDaemonRejectPlan)
		})

	})
}
