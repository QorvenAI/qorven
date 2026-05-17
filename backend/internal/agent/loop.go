// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the FSL-1.1-ALv2 license
// that can be found in the LICENSE file.

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/billing"
	"github.com/qorvenai/qorven/internal/connectors"
	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/plugin"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/skills"
	supervisorpkg "github.com/qorvenai/qorven/internal/supervisor"
	"github.com/qorvenai/qorven/internal/token"
	"github.com/qorvenai/qorven/internal/tools"
	"github.com/qorvenai/qorven/internal/webintel"
)

// Loop is the core agent execution engine.
// It implements the think→act→observe cycle with streaming.
type Loop struct {
	agentStore      *Store
	sessionStore    *session.Store
	providerReg     *providers.Registry
	toolReg         *tools.Registry
	skillLoader     *skills.Loader
	skillStore      *skills.Store
	memStore        *memory.Store
	connKB          *connectors.KnowledgeStore
	mcpMgr          *mcp.Manager
	billingStore    *billing.Store
	bundleStore     *BundleStore
	tenantID        string
	Hooks           *HookChain
	Crystallizer    *skills.Crystallizer
	SmartRouter     *providers.SmartRouter
	ModelSwitchQ    *providers.ModelSwitchQueue
	BackgroundModel string              // cheap/fast model for title, tags, intent — avoids burning expensive tokens on background tasks
	WebAugmenter    *webintel.Augmenter // Perplexity-style web search augmentation
	OnMessage       func(sessionID, agentID, role, content, channel string)
	Events          *EventBus
	LearningLoop    *LearningLoop
	SupervisorBus   *supervisorpkg.Bus                              // inter-agent protocol
	PrimeID         string                                          // Prime's agent ID for heartbeats
	HierarchyMem    *memory.HierarchyStore                          // company > team > agent memory
	WorkingMem      *memory.WorkingMemory                           // short-term events
	KnowledgeGraph  *memory.KnowledgeGraph                          // entity + relationship graph
	systemKnowledge string                                          // QORVEN.md content for Prime
	skillLearner    *skills.Learner                                 // self-improving skill creation
	pluginMgr       *plugin.Manager                                 // plugin system
	primeDelegation *PrimeDelegation                                // Prime → specialist delegation
	promptCache     *PromptCache                                    // system prompt cache
	projectReg      *tools.ProjectRegistry                          // code project registry
	auditFn         func(agent, tool, session string, isError bool) // tool execution audit callback
	// PIIRedactor is invoked on inbound user messages and outbound
	// assistant content. nil = disabled (default). Set via
	// SetPIIRedactor from the gateway after reading the tenant config.
	PIIRedactor PIIRedactor

	// PromptGuardPolicy decides whether to scan user messages for
	// prompt-injection attempts and what to do on a hit. PromptGuardOff
	// (default) = no scanning. Set via SetPromptGuardPolicy.
	PromptGuardPolicy PromptInjectionPolicy

	// PermGate seeds per-Qor permission profiles at session start.
	// nil = feature disabled (safe default).
	PermGate *permissions.Gate
}

// PIIRedactor decides what to do with inbound/outbound text. Kept as
// an interface so the agent package doesn't depend on internal/pii
// directly — avoids any risk of an import cycle and lets tests mock
// redaction without setting up the regex engine.
type PIIRedactor interface {
	Redact(text string) string
}

// SetPIIRedactor wires the PII filter; a nil arg disables redaction.
// Safe to call before Run — but not concurrently from multiple
// goroutines, since assignment is a simple field write and Run may be
// reading it from another goroutine. Call once at startup.
func (l *Loop) SetPIIRedactor(r PIIRedactor) { l.PIIRedactor = r }

// GetTenantID exposes the tenant ID for engine-level recovery routines.
func (l *Loop) GetTenantID() string { return l.tenantID }

// resolveTenantAdminUserID returns the UUID of the tenant's admin/owner user.
// Used as a fallback when req.UserID is absent or is a non-UUID channel sender ID
// (e.g. Telegram numeric IDs) so the permission gate can find auto-approved policies.
// Returns "" if the DB is unavailable or no admin user exists.
func (l *Loop) resolveTenantAdminUserID(ctx context.Context) string {
	if l.agentStore == nil || l.agentStore.Pool() == nil || l.tenantID == "" {
		return ""
	}
	var userID string
	l.agentStore.Pool().QueryRow(ctx,
		`SELECT id::text FROM users WHERE tenant_id = $1::uuid AND is_active = true ORDER BY created_at LIMIT 1`,
		l.tenantID,
	).Scan(&userID) //nolint:errcheck
	return userID
}

var errNoProvider = fmt.Errorf("no provider configured")

type providerError struct{ msg string }
type requestIDKey struct{}

func NewLoop(
	agentStore *Store,
	sessionStore *session.Store,
	providerReg *providers.Registry,
	toolReg *tools.Registry,
	skillLoader *skills.Loader,
	memStore *memory.Store,
	tenantID string,
) *Loop {
	return &Loop{
		agentStore:   agentStore,
		sessionStore: sessionStore,
		providerReg:  providerReg,
		toolReg:      toolReg,
		skillLoader:  skillLoader,
		memStore:     memStore,
		tenantID:     tenantID,
	}
}

// Run executes the full agent loop for a request.
// onEvent is called for each streaming event (text chunks, tool calls, etc.).
// The loop: build context → call LLM → if tool calls, execute and loop → return final text.
func (l *Loop) Run(ctx context.Context, req RunRequest, onEvent func(StreamEvent)) (*RunResult, error) {
	start := time.Now()

	// Nil-guard: callers like QOROS's Engine.StartQoros and the supervisor
	// loop fire Run with onEvent=nil because they don't care about
	// streaming. The big branch of onEvent(...) calls below doesn't
	// nil-check, so a QOROS tick that triggers a tool call panics with
	// "nil pointer dereference" at agent/loop.go:949 (onEvent(ToolStart)).
	// Replacing nil with a no-op callback keeps the hot path clean
	// without adding 30+ `if onEvent != nil` checks.
	if onEvent == nil {
		onEvent = func(StreamEvent) {}
	}

	// CRITICAL: Always send DoneEvent, even on panic/error — prevents "thinking forever" in UI
	doneSent := false
	defer func() {
		if !doneSent {
			doneSent = true
			onEvent(DoneEvent())
		}
	}()

	// Run pre-hooks
	if l.Hooks != nil {
		if err := l.Hooks.RunPre(ctx, &req); err != nil {
			return nil, err
		}
	}

	// 0. Normalize user input — fix typos, STT artifacts
	if req.UserMessage != "" {
		req.UserMessage = NormalizeQuery(req.UserMessage)
		// Limit message length to prevent context overflow
		if len(req.UserMessage) > 100000 {
			req.UserMessage = req.UserMessage[:100000] + "\n[message truncated — 100K char limit]"
		}
		// PII redaction happens AFTER normalisation so the regex detectors
		// see the canonical form, and BEFORE the message enters the
		// message history (which is persisted + sent to the LLM).
		if l.PIIRedactor != nil {
			req.UserMessage = l.PIIRedactor.Redact(req.UserMessage)
		}
		// Prompt-injection scan runs AFTER PII redaction on purpose:
		// the scan sees the same text the LLM would see. If we scanned
		// first, an attacker could hide an injection inside a PII
		// pattern (rare but possible). Post-redaction scanning also
		// means a "{{PII:email}}" marker doesn't itself look like an
		// encoded payload to the detector.
		if pgr := l.scanPromptInjection(req.UserMessage); pgr != nil && pgr.Block {
			// Short-circuit: return the canned refusal as the agent's
			// reply. No LLM call, no tool execution, no memory write.
			onEvent(StreamEvent{Type: "prompt_guard_blocked", Data: map[string]any{
				"score":      pgr.Report.Score,
				"detections": pgr.Report.Detections,
			}})
			return &RunResult{
				Content: pgr.UserMessage,
				Parts:   []MessagePart{TextPart(pgr.UserMessage)},
			}, nil
		}
	}

	// 1. Load agent (try by key first, then by UUID)
	ag, err := l.agentStore.GetByKey(ctx, req.AgentID)
	if err != nil {
		ag, err = l.agentStore.Get(ctx, req.AgentID)
		if err != nil {
			return nil, err
		}
	}

	// 1b. Seed default permission profile for this Qor (idempotent — ON CONFLICT DO NOTHING).
	// For channel messages (Telegram, WhatsApp), req.UserID may be a non-UUID sender ID.
	// Fall back to the tenant admin user so the permission gate can find auto-approved policies.
	if l.PermGate != nil {
		effectiveUserID := req.UserID
		if _, err := uuid.Parse(effectiveUserID); err != nil {
			effectiveUserID = l.resolveTenantAdminUserID(ctx)
		}
		if effectiveUserID != "" {
			if err := l.PermGate.LoadDefaults(ctx, l.tenantID, effectiveUserID, ag.ID, ag.AgentKey); err != nil {
				slog.Warn("agent.loop.perm_defaults_failed", "agent", ag.ID, "error", err)
			}
		}
	}

	// 1c. Apply role constraints (structural enforcement — overrides DB config).
	// This runs before BuildToolDefs so the tool filter sees the role's surface,
	// not whatever the DB had. rolePromptMode is non-zero only when the role
	// mandates a specific prompt assembly mode (e.g. PromptIntake for prime).
	//
	// Exception: plan_graph channel is an orchestrator-internal call. We never
	// apply intake role restrictions on that path — the orchestrator needs the
	// full tool surface even when it invokes the "prime" agent key as the planner.
	var rolePromptMode PromptMode
	if role, ok := ResolveRole(ag.AgentKey); ok && req.Channel != "plan_graph" {
		rolePromptMode = ApplyRole(ag, role)
		slog.Debug("agent.role.applied", "agent", ag.AgentKey, "role_mode", rolePromptMode,
			"max_iter", ag.MaxToolIterations)
	}

	// 2. Resolve provider
	provider := l.resolveProvider(ag)
	if provider == nil {
		return nil, errNoProvider
	}

	// 3. Load session history
	history := l.loadHistory(ctx, req.SessionID)

	// Gap A fix: Unified agent thread — if this is a fresh session (no history),
	// inject a continuation summary from the agent's recent sessions across ALL channels.
	// This gives the agent "memory" of what it was doing on web, Telegram, cron, etc.
	// Never shown to the user — injected as a silent system message at position 0.
	if len(history) == 0 && l.sessionStore != nil && req.SessionID != "" &&
		req.Channel != "cron" && req.Channel != "btw" && req.Channel != "subagent" && req.Channel != "intake" {
		continuation := l.sessionStore.GetContinuationSummary(ctx, ag.ID, 3)
		if continuation != "" {
			history = []providers.Message{{
				Role:    "system",
				Content: continuation,
			}}
			slog.Debug("context.continuation_injected", "agent", ag.ID, "channel", req.Channel)
		}
	}

	// Smart history compression: keep recent messages, summarize older ones
	// Uses token estimation to decide when to compress
	if len(history) > 20 {
		keepRecent := 12
		if len(history) > 40 {
			keepRecent = 8
		}
		recent := history[len(history)-keepRecent:]
		older := history[:len(history)-keepRecent]
		// Build structured summary of older messages
		var summary strings.Builder
		summary.WriteString("[Conversation context — ")
		summary.WriteString(fmt.Sprintf("%d earlier messages summarized]\n", len(older)))
		for _, m := range older {
			switch m.Role {
			case "user":
				preview := m.Content
				if len(preview) > 120 {
					preview = preview[:120] + "..."
				}
				summary.WriteString("• User: " + preview + "\n")
			case "assistant":
				preview := m.Content
				if len(preview) > 120 {
					preview = preview[:120] + "..."
				}
				summary.WriteString("• Assistant: " + preview + "\n")
			}
		}
		history = append([]providers.Message{{Role: "system", Content: summary.String()}}, recent...)
		slog.Debug("context.compressed", "agent", ag.ID, "kept", keepRecent, "summarized", len(older))
	}

	// 3b. Prompt cache: reuse system prompt if unchanged
	var systemPrompt string
	promptCacheHit := false
	if l.promptCache != nil {
		if cached, ok := l.promptCache.Get(ag.ID, req.SessionID); ok {
			systemPrompt = cached
			promptCacheHit = true
			slog.Debug("prompt.cache.hit", "agent", ag.ID, "session", req.SessionID)
		}
	}

	// 4. Search relevant memories — bounded 200ms deadline so a slow DB
	// never delays the LLM call.  Partial results are used on timeout;
	// errors are logged (not silenced) so degradation is observable.
	var memResults []memory.SearchResult
	if !promptCacheHit && req.UserMessage != "" {
		memCtx, memCancel := context.WithTimeout(ctx, 200*time.Millisecond)
		var memErr error
		if l.HierarchyMem != nil {
			if len(req.MemoryScopeAllow) > 0 || len(req.MemoryScopeDeny) > 0 {
				memResults, memErr = l.HierarchyMem.SearchHierarchyScoped(memCtx, ag.ID, "", req.UserMessage, 8, req.MemoryScopeAllow, req.MemoryScopeDeny)
			} else {
				memResults, memErr = l.HierarchyMem.SearchHierarchy(memCtx, ag.ID, "", req.UserMessage, 8)
			}
		} else if l.memStore != nil {
			memResults, memErr = l.memStore.Search(memCtx, l.tenantID, ag.ID, req.UserMessage, 5)
		}
		memCancel()
		if memErr != nil && memErr != context.DeadlineExceeded && memErr != context.Canceled {
			slog.Warn("agent.loop.memory_search_error", "agent", ag.ID, "error", memErr)
		} else if memErr != nil {
			slog.Debug("agent.loop.memory_partial_recall", "agent", ag.ID, "partial", len(memResults))
		}
	}
	// 5. Get memory bulletin
	var bulletin string
	if l.memStore != nil {
		bulletin, _ = l.memStore.GetLatestBulletin(ctx, ag.ID)
	}

	// 6. Build context
	cb := NewContextBuilder(ag, l.skillLoader, l.memStore, l.toolReg)
	// propagate per-request extra tools (tenant-scoped
	// Wasm plugins injected by the orchestrator) so BuildToolDefs
	// includes them in the LLM's offered tool list.
	if len(req.ExtraTools) > 0 {
		cb.SetExtraTools(req.ExtraTools)
	}
	if !promptCacheHit {
		if l.skillStore != nil {
			cb.SetSkillStore(l.skillStore)
		}

		// Set runtime context from request
		rc := RuntimeContext{Mode: PromptFull, Channel: "dm", TriggerBy: "user"}
		if req.Channel == "room" {
			rc.Channel = "room"
			rc.TriggerBy = "mention"
		} else if req.Channel == "delegation" {
			rc.Mode = PromptMinimal
			rc.Channel = "delegation"
			rc.TriggerBy = "delegation"
		} else if req.Channel == "cron" {
			rc.Mode = PromptCron
			rc.Channel = "cron"
			rc.TriggerBy = "cron"
		} else if req.Channel == "intake" {
			rc.Mode = PromptIntake
			rc.Channel = "intake"
			rc.TriggerBy = "user"
		} else if req.Channel != "" && req.Channel != "web" {
			rc.Mode = PromptChannel
			rc.Channel = req.Channel
			rc.TriggerBy = "channel"
		}
		// Role PromptMode wins over channel-derived mode (role is structural, channel is contextual).
		if rolePromptMode != "" {
			rc.Mode = rolePromptMode
		}
		cb.SetRuntime(rc)

		// Inject team roster
		if l.agentStore != nil {
			if roster, err := l.agentStore.List(ctx, l.tenantID); err == nil && len(roster) > 1 {
				cb.SetTeamRoster(roster)
			}
		}
		// Pass memory results to PromptBuilder for section 9
		if len(memResults) > 0 {
			memStrs := make([]string, len(memResults))
			for i, m := range memResults {
				memStrs[i] = m.Memory.Content
			}
			cb.SetMemoryResults(memStrs)
		}
		// Inject learned preferences from learning loop
		if l.LearningLoop != nil {
			if hints := l.LearningLoop.GetLearnedHints(ag.ID); hints != "" {
				cb.SetLearnedHints(hints)
			}
		}
		systemPrompt = cb.BuildSystemPrompt(bulletin)
		if systemPrompt == "" {
			systemPrompt = "You are a helpful AI assistant."
		}

		// Prime Coder mode: inject structured coding workflow for code sessions.
		// Runs for both "code-*" sessions (direct /code projects) and inception agents
		// whose sessions are normal UUIDs but whose ProjectBriefID links them to a workspace.
		if req.Channel != "intake" {
			projectPath := ""
			if l.projectReg != nil {
				// Primary: session-based lookup for direct /code projects
				for _, p := range l.projectReg.List() {
					if p.SessionID == req.SessionID {
						projectPath = p.Path
						break
					}
				}
				// Fallback: inception agents whose session is not "code-*"
				if projectPath == "" && ag.ProjectBriefID != "" {
					if p := l.projectReg.GetByBriefID(ag.ProjectBriefID); p != nil {
						projectPath = p.Path
					}
				}
			}
			if projectPath != "" {
				systemPrompt += "\n\n" + PrimeCoderSystemPrompt(projectPath)
			}

			// Load project-scoped memories (task scope)
			if l.HierarchyMem != nil && projectPath != "" {
				taskResults, _ := l.HierarchyMem.SearchTask(context.Background(), req.SessionID, req.UserMessage, 5)
				if len(taskResults) > 0 {
					systemPrompt += "\n\n## Project Memory\n"
					for _, r := range taskResults {
						systemPrompt += "- " + r.Memory.Content + "\n"
					}
				}
			}
		}

		// Cache the built prompt for reuse on next turn
		if l.promptCache != nil && systemPrompt != "" {
			l.promptCache.Set(ag.ID, req.SessionID, systemPrompt)
		}
	} // end if !promptCacheHit

	// Plan mode: inject planning-only system prompt
	if req.Mode == "plan" {
		systemPrompt += "\n\n" + PlanModeSystemPrompt
	}

	// Supervisor protocol: inject awareness when Prime is active
	if l.SupervisorBus != nil && l.PrimeID != "" && ag.ID != l.PrimeID {
		systemPrompt += "\n\n" + SupervisorProtocolPrompt
	}

	// Inject curated memory from Engine (frozen snapshot — cache-stable)
	if req.MemoryBulletin != "" {
		systemPrompt += "\n\n" + req.MemoryBulletin
	}
	if req.UserProfile != "" {
		systemPrompt += "\n\n" + req.UserProfile
	}
	if req.WorkingMemory != "" {
		systemPrompt += "\n\n" + req.WorkingMemory
	}

	// Inject live working memory (recent events from this session)
	if l.WorkingMem != nil {
		if wmPrompt := l.WorkingMem.ForSystemPrompt(); wmPrompt != "" {
			systemPrompt += "\n\n## Recent Events\n" + wmPrompt
		}
	}

	// Inject per-agent instruction bundles (SOUL, TOOLS, IDENTITY)
	if l.bundleStore != nil {
		if section := l.bundleStore.BuildPromptSection(ctx, ag.ID); section != "" {
			systemPrompt += section
		}
	}

	// Inject connected service knowledge (Gmail, Slack, HubSpot, etc.)
	if l.connKB != nil {
		if k := IntegrationKnowledge(ctx, l.tenantID, ag.ID, l.connKB, l.mcpMgr); k != "" {
			systemPrompt += "\n" + k
		}
	}

	// Inject user profile context
	if l.agentStore != nil && l.agentStore.Pool() != nil {
		var facts, prefs string
		var tz, lang string
		l.agentStore.Pool().QueryRow(ctx, `SELECT COALESCE(facts::text,'{}'), COALESCE(preferences::text,'{}'), COALESCE(timezone,''), COALESCE(language,'en') FROM user_profiles WHERE tenant_id = $1 LIMIT 1`, l.tenantID).Scan(&facts, &prefs, &tz, &lang)
		if facts != "{}" || tz != "" {
			var profile map[string]any
			json.Unmarshal([]byte(facts), &profile)
			if profile == nil {
				profile = make(map[string]any)
			}
			if tz != "" {
				profile["timezone"] = tz
			}
			if lang != "" {
				profile["language"] = lang
			}
			var prefsMap map[string]any
			json.Unmarshal([]byte(prefs), &prefsMap)
			if prefsMap != nil {
				profile["preferences"] = prefsMap
			}
			systemPrompt = InjectUserProfile(systemPrompt, profile)
		}
	}

	// OpenSpace: inject crystallized skills (proven procedures) before LLM runs
	if l.Crystallizer != nil && req.UserMessage != "" {
		crystSkills, _ := l.Crystallizer.SearchSimilar(ctx, ag.ID, req.UserMessage, 2)
		if len(crystSkills) > 0 {
			var sb strings.Builder
			sb.WriteString(systemPrompt)
			sb.WriteString("\n\n## Proven Procedures (reuse these)\n")
			for _, cs := range crystSkills {
				sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", cs.Name, cs.Procedure))
				go l.Crystallizer.IncrementReuse(context.Background(), cs.ID)
			}
			systemPrompt = sb.String()
			slog.Info("skill.retrieval.injected", "agent", ag.AgentKey, "skills", len(crystSkills))
		}
	}

	messages := cb.BuildMessages(history, req.UserMessage, memResults)

	// Stable-prefix cache breakpoint: build a cached system block for sections that
	// don't change per-turn (platform, rules, safety, tools posture).
	// This is prepended in the inner run loop alongside the main system prompt.
	stableSystemMsg := cb.BuildStableSystem()

	// === FILE EXTRACTION ===
	// Separate <attached_file> blocks from user message into structured context
	// This prevents the LLM from echoing file content back
	var extractedFiles []ExtractedFile
	if req.UserMessage != "" {
		cleanMsg, files := ExtractFilesFromMessage(req.UserMessage)
		if len(files) > 0 {
			extractedFiles = files
			// Replace the last message (user) with clean message
			if len(messages) > 0 {
				messages[len(messages)-1] = providers.Message{Role: "user", Content: cleanMsg}
			}
			// Inject file context as separate message before user message
			fileCtx := BuildFileContextMessage(files)
			// Insert before last message
			last := messages[len(messages)-1]
			messages = append(messages[:len(messages)-1], fileCtx,
				providers.Message{Role: "assistant", Content: "I'll analyze the attached file(s) now."},
				last)
			// Update req.UserMessage for downstream (intent, widget detection)
			req.UserMessage = cleanMsg
		}
	}
	_ = extractedFiles // used for future file-specific processing

	// === PIPELINE ENRICHMENT (the Perplexity Effect) ===
	// Mentor-approved order: intent → memory → RAG → tool gating
	var chatIntent ChatIntent
	if req.UserMessage != "" {
		chatIntent = ClassifyChatIntent(req.UserMessage)
		slog.Info("pipeline.intent", "agent", ag.AgentKey, "intent", chatIntent)

		// Batch routing: non-streaming research/analysis → batch API for 50% savings
		if !req.Stream && providers.ShouldBatch(string(chatIntent), false) && req.Depth == "balanced" {
			slog.Info("pipeline.batch_candidate", "agent", ag.AgentKey, "intent", chatIntent)
			// For now, log the candidate — full batch routing requires async job tracking
			// Batch routing: queue for async processing if agent supports it
			if ag.Model != "" && strings.Contains(ag.Model, "batch") {
				if gw, ok := ctx.Value("gateway").(*interface{}); ok && gw != nil {
					slog.Info("pipeline.batch_queued", "agent", ag.AgentKey)
				}
			}
		}

		// Enrich messages with RAG context and memory (injected as separate user messages)
		messages = EnrichMessages(ctx, messages, req.UserMessage, chatIntent, memResults, nil)
	}

	// Web Intelligence: auto-augment with web search (Perplexity-style)
	// Widget detection (weather, calculation) — deterministic, no LLM needed
	if ag.WebSearchEnabled && req.UserMessage != "" {
		if widget := webintel.DetectWidget(req.UserMessage); widget != nil {
			if widget.Type == "weather" {
				loc := extractLocation(req.UserMessage)
				onEvent(StreamEvent{Type: "tool_start", Data: "Fetching weather for " + loc})
				if wxData, err := webintel.FetchWeather(ctx, loc); err == nil {
					onEvent(StreamEvent{Type: "tool_result", Data: map[string]any{"name": "weather", "result": fmt.Sprintf("%v, %v", wxData["location"], wxData["condition"])}})
					onEvent(WidgetEvent(map[string]any{"type": "weather", "data": wxData}))
					onEvent(PartEvent(WidgetPart("weather", wxData)))
					loc := fmt.Sprintf("%v", wxData["location"])
					cond := fmt.Sprintf("%v", wxData["condition"])
					humid := toF64(wxData["humidity"])
					wind := toF64(wxData["wind_speed"])
					hi := toF64(wxData["temperature"]) + 2
					lo := toF64(wxData["temperature"]) - 5
					if d, ok := wxData["daily"].(map[string]any); ok {
						if maxT, ok := d["temperature_2m_max"].([]any); ok && len(maxT) > 0 {
							hi = toF64(maxT[0])
						}
						if minT, ok := d["temperature_2m_min"].([]any); ok && len(minT) > 0 {
							lo = toF64(minT[0])
						}
					}
					temp := toF64(wxData["temperature"])
					shortText := fmt.Sprintf("%s is currently **%.0f°C** with **%s** conditions. Winds at **%.0f km/h** and humidity at **%.0f%%**.\n\nFor today, a high of **%.0f°C** and a low of **%.0f°C** are expected.\n\n",
						loc, temp, strings.ToLower(cond), wind, humid, hi, lo)
					if temp > 30 {
						shortText += "Would you like me to find indoor activities or cool spots nearby?"
					} else if temp < 10 {
						shortText += "Would you like me to find warm cafés or indoor activities nearby?"
					} else if humid > 70 || strings.Contains(strings.ToLower(cond), "rain") {
						shortText += "Would you like me to find indoor spots to stay dry today?"
					} else {
						shortText += "Would you like me to find outdoor activities or things to do there?"
					}
					onEvent(TextDelta(shortText))
					onEvent(PartEvent(TextPart(shortText)))
					wxResult := &RunResult{Content: shortText, Parts: []MessagePart{WidgetPart("weather", wxData), TextPart(shortText)}, Metadata: map[string]any{"widgets": []any{map[string]any{"type": "weather", "data": wxData}}}}
					doneSent = true
					onEvent(DoneEvent())
					l.saveToSession(ctx, req, wxResult)
					return wxResult, nil
				}
			}
		}
	}

	// Detect additional widgets (timezone, currency, units, YouTube)
	for _, w := range webintel.DetectAllWidgets(ctx, req.UserMessage) {
		onEvent(WidgetEvent(map[string]any{"type": w.Type, "data": w.Params}))
		onEvent(PartEvent(WidgetPart(w.Type, w.Params)))
	}
	toolDefs := cb.BuildToolDefs()
	// Pipeline: gate tools by intent (mentor: don't include web_search for chat/creative)
	// Skip gating for self_build, cron, and task channels — they need full tool access
	// (task channel injects lifecycle tools that are not in intentTools)
	if req.UserMessage != "" && req.Channel != "self_build" && req.Channel != "cron" && req.Channel != "task" {
		toolDefs = GateToolsByIntent(toolDefs, chatIntent)
		slog.Info("agent.loop.tools_gated", "agent", ag.AgentKey, "intent", chatIntent, "tools", len(toolDefs))
	}
	// Plan mode: restrict to read-only tools
	if req.Mode == "plan" {
		toolDefs = FilterPlanModeTools(toolDefs)
	}
	// Explicit delegation mode: strip auto-delegation tools so the LLM
	// cannot delegate autonomously — only @-mention commands fire delegation.
	if req.DelegationMode == "explicit" {
		filtered := toolDefs[:0]
		for _, td := range toolDefs {
			if td.Function.Name != "delegate_to_soul" && td.Function.Name != "spawn_agent" {
				filtered = append(filtered, td)
			}
		}
		toolDefs = filtered
	}
	toolCtx := tools.WithWorkspace(ctx, tools.AgentWorkspace(ag.ID))
	toolCtx = tools.WithAgentID(toolCtx, ag.ID)
	toolCtx = tools.WithSessionID(toolCtx, req.SessionID)
	toolCtx = tools.WithDiscussionID(toolCtx, req.DiscussionID)
	// Wire fork subagent so spawn tool can delegate
	toolCtx = tools.WithForkFunc(toolCtx, func(forkCtx context.Context, directive string) (string, error) {
		forkReq := ForkRequest{
			ParentSessionID: req.SessionID,
			AgentID:         ag.ID,
			ForkID:          fmt.Sprintf("%d", time.Now().UnixMilli()),
			Directive:       directive,
			Config:          DefaultForkConfig,
			OnEvent:         onEvent,
		}
		result, err := l.ForkSubagent(forkCtx, forkReq)
		if err != nil {
			return "", err
		}
		return result.Content, nil
	})

	// 8. Think→Act→Observe loop
	result := &RunResult{TraceID: uuid.New().String()}
	maxIter := ag.MaxToolIterations
	// Cap iterations for simple queries
	if chatIntent == ChatIntentChat && maxIter > 5 {
		maxIter = 5
	}
	if chatIntent == ChatIntentResearch && maxIter > 8 {
		maxIter = 8
	}
	if maxIter <= 0 {
		maxIter = 10
	}
	// Cron tasks: cap at 3 iterations to prevent runaway tool loops
	if req.Channel == "cron" && maxIter > 3 {
		maxIter = 3
	}

	model := req.Model
	if model == "" {
		model = ag.Model
	}
	primaryModel := model // QORVEN: track primary for restoration after fallback
	usedFallback := false

	// Model switch queue: apply pending switches, mark agent as busy
	if l.ModelSwitchQ != nil {
		// model = l.ModelSwitchQ.MarkBusy(ag.ID, model) // DISABLED: use agent configured model
		defer l.ModelSwitchQ.MarkDone(ag.ID)
	}

	// Smart Router: classify query and route to best model per work category
	if l.SmartRouter != nil && req.UserMessage != "" {
		decision, err := l.SmartRouter.ClassifyAndRoute(ctx, l.tenantID, req.UserMessage)
		if err == nil && decision.ModelID != "" {
			slog.Info("agent.loop.smart_route", "agent", ag.AgentKey, "category", decision.Category,
				"confidence", decision.Confidence, "from", model, "to", decision.ModelID)
			// Only switch model if agent is set to "auto" routing
			// AND the target model exists on a configured provider
			if ag.Model == "auto" || ag.Model == "" {
				if l.providerReg != nil && l.providerReg.HasModel(decision.ModelID) {
					model = decision.ModelID
				} else {
					slog.Warn("smart_router.model_not_available", "model", decision.ModelID, "keeping", model)
				}
			}
		}
	}

	// Use model's actual context window as an upper bound on the agent's configured window.
	contextWindow := ag.ContextWindow
	if l.providerReg != nil {
		if modelLimit := l.providerReg.GetModelContextWindow(model); modelLimit > 0 && modelLimit < contextWindow {
			slog.Warn("agent.context_window_exceeds_model_limit",
				"agent", ag.ID, "configured", contextWindow,
				"model_limit", modelLimit, "using", modelLimit)
			contextWindow = modelLimit
		}
	}

	// Mid-loop compactor (Qorven-inspired: compact at 75% context during iterations)
	compactor := NewCompactor(contextWindow)

	// Brain components (from BRAIN-ARCHITECTURE-v2)
	loopGuard := NewLoopGuard()
	scrubber := NewSecretScrubber(l.loadSecretPatterns())
	shellSec := NewShellSecurity()

	// Pre-agent hygiene: compress before agent starts (Qorven pattern)
	action := compactor.Check(messages)
	if action != NoCompaction {
		slog.Info("agent.loop.pre_hygiene", "action", action, "msgs", len(messages))
		messages = DedupToolResults(messages)
		if pruned, savings := compactor.PruneTools(messages); savings > 0 {
			messages = pruned
		}
		if action >= BackgroundCompaction {
			// Memory flush before compaction — save key facts so they survive summarization
			if l.memStore != nil && ag.MemoryEnabled {
				l.flushMemoryBeforeCompaction(ctx, provider, ag, messages)
			}
			messages = compactor.Compact(messages, action)
		}
	}

	// Create a set of allowed tool names for enforcement
	allowedTools := make(map[string]bool)
	for _, td := range toolDefs {
		allowedTools[td.Function.Name] = true
	}
	slog.Info("agent.loop.allowed_tools", "agent", ag.AgentKey, "tools", len(allowedTools))

	// Track web_search and web_fetch calls for budget enforcement
	webSearchCalls := 0
	webFetchCalls := 0

	var lastLLMContent string
	consecutiveToolIters := 0 // track iterations with only tool calls, no text
	for iter := 0; iter < maxIter; iter++ {
		// Search discipline: after 5 tool-only iterations, force the agent to answer
		// Check if request was cancelled (user stopped, timeout)
		if ctx.Err() != nil {
			result.Content = "Request cancelled."
			break
		}
		if consecutiveToolIters >= 5 {
			slog.Warn("agent.loop.search_discipline", "agent", ag.ID, "forcing_answer", true, "tool_iters", consecutiveToolIters)
			messages = append(messages, providers.Message{
				Role:    "user",
				Content: "STOP using tools. You have done enough research. Answer the question NOW with the information you already have. Do NOT call any more tools.",
			})
			consecutiveToolIters = 0
		}

		// QORVEN: Restore primary model after fallback
		// On each new turn, try the primary model first instead of staying on fallback
		if usedFallback && iter > 0 {
			slog.Info("agent.loop.restore_primary", "agent", ag.ID, "from", model, "to", primaryModel)
			model = primaryModel
			usedFallback = false
		}

		// Budget enforcement: check before each LLM call
		if l.billingStore != nil {
			if err := l.billingStore.EnforceBudget(ctx, ag.ID); err != nil {
				result.Content = "⚠️ " + err.Error()
				break
			}
		}
		result.Iterations = iter + 1

		// Mid-loop compaction: if messages are too large, compact before calling LLM
		if iter > 0 {
			// Tier 1: Dedup + prune tool outputs (free, no LLM)
			messages = DedupToolResults(messages)
			action := compactor.Check(messages)
			if action == PruneToolOutputs {
				if pruned, savings := compactor.PruneTools(messages); savings > 0 {
					messages = pruned
				}
			} else if action != NoCompaction {
				// Tier 1 first, then Tier 2/3
				if pruned, savings := compactor.PruneTools(messages); savings > 0 {
					messages = pruned
				}
				messages = compactor.Compact(messages, action)
			}

			// Circuit breaker check
			if det := loopGuard.DetectErrorCircuitBreak(); det.Level == DetectionCritical {
				slog.Error("agent.loop.circuit_break", "agent", ag.ID, "msg", det.Message)
				result.Content = det.Message
				break
			}
		}

		// Build the full message list with system prompt.
		// Two-block system: stable (cacheable) prefix + dynamic (per-turn) suffix.
		// Anthropic: two separate text blocks with cache_control on the stable one.
		// Other providers: both system messages are concatenated by the provider layer.
		buildFullMessages := func(sysMsgs ...string) []providers.Message {
			out := make([]providers.Message, 0, len(sysMsgs)+len(messages))
			if stableSystemMsg.Content != "" {
				out = append(out, stableSystemMsg)
			}
			for _, s := range sysMsgs {
				if s != "" {
					out = append(out, providers.Message{Role: "system", Content: s})
				}
			}
			return append(out, messages...)
		}
		fullMessages := buildFullMessages(systemPrompt)

		// Pre-flight token check: auto-compact if approaching context limit
		tc := &token.Counter{}
		if tc.WillExceedLimit(fullMessages, contextWindow) {
			slog.Warn("agent.loop.token_preflight", "agent", ag.ID, "est_tokens", tc.Estimate(fullMessages), "limit", contextWindow)
			// Tier 1: compact messages
			messages = compactor.Compact(messages, AggressiveCompaction)
			fullMessages = buildFullMessages(systemPrompt)
			// Tier 2: if still too big, compress system prompt (remove tool routing guide, trim descriptions)
			if tc.WillExceedLimit(fullMessages, contextWindow) {
				slog.Warn("agent.loop.system_prompt_compress", "agent", ag.ID, "prompt_chars", len(systemPrompt))
				systemPrompt = compressSystemPrompt(systemPrompt, contextWindow/4)
				fullMessages = buildFullMessages(systemPrompt)
			}
		}

		// Tool enforcement: retry with "required" if model said "I'll search" without calling tools
		toolChoice := "auto"
		if lastLLMContent != "" && looksLikeToolIntent(lastLLMContent) && len(toolDefs) > 0 {
			toolChoice = "required"
			slog.Info("agent.loop.enforce_tools", "agent", ag.ID, "iter", iter)
		}

		chatOpts := map[string]any{"temperature": autoTemperature(req.UserMessage, iter), "max_tokens": 4096}
		effectiveThinkingLevel := ag.ThinkingLevel
		if req.ThinkingLevel != "" {
			effectiveThinkingLevel = req.ThinkingLevel // per-request overrides agent default
		}
		if effectiveThinkingLevel != "" && effectiveThinkingLevel != "off" {
			decision := providers.ResolveReasoningDecision(provider, model, effectiveThinkingLevel, providers.ReasoningFallbackDowngrade, "thinking_level")
			if decision.EffectiveEffort != "" && decision.EffectiveEffort != "off" {
				applyReasoningToOptions(chatOpts, provider, model, decision.EffectiveEffort)
			}
		}

		chatReq := providers.ChatRequest{
			Model:      model,
			Messages:   fullMessages,
			Tools:      toolDefs,
			ToolChoice: toolChoice,
			Options:    chatOpts,
		}

		// Call LLM with streaming + repetition detection
		var llmResp *providers.ChatResponse
		var streamedChars int
		var lastChunk string
		var repeatCount int
		var accumulated strings.Builder
		const maxStreamChars = 16000 // 16KB max output (was 50KB — too much for repetition)
		const maxRepeatChunks = 3    // stop after 3 identical chunks

		if ctx.Err() != nil {
			result.Content = "Request cancelled."
			break
		}
		llmResp, err = provider.ChatStream(ctx, chatReq, func(chunk providers.StreamChunk) {
			if chunk.Content != "" {
				streamedChars += len(chunk.Content)
				accumulated.WriteString(chunk.Content)

				// Repetition detection: identical chunks
				if chunk.Content == lastChunk && len(chunk.Content) > 20 {
					repeatCount++
					if repeatCount >= maxRepeatChunks {
						slog.Warn("agent.loop.repetition_detected", "agent", ag.ID, "type", "chunk", "repeats", repeatCount)
						return
					}
				} else {
					repeatCount = 0
					lastChunk = chunk.Content
				}

				// Sentence-level repetition: check if same sentence appears 3+ times
				acc := accumulated.String()
				if len(acc) > 200 {
					// Find the last sentence
					lastDot := strings.LastIndex(acc[:len(acc)-1], ".")
					if lastDot > 50 {
						lastSentence := strings.TrimSpace(acc[lastDot-50 : lastDot+1])
						if len(lastSentence) > 30 && strings.Count(acc, lastSentence) >= 3 {
							slog.Warn("agent.loop.repetition_detected", "agent", ag.ID, "type", "sentence", "sentence", lastSentence[:40])
							return
						}
					}
				}

				// Max output guard
				if streamedChars > maxStreamChars {
					return
				}

				// Filter raw tool call markup from streaming (DeepSeek/Qwen native format)
				if strings.Contains(chunk.Content, "<|tool_call") ||
					strings.Contains(chunk.Content, "tool_calls_section") ||
					strings.Contains(chunk.Content, "<|plugin") ||
					strings.Contains(chunk.Content, "functions.") {
					return // skip — this is a tool call, not user-visible text
				}

				// Suppress Gemini tool_code blocks from streaming to user
				if strings.Contains(accumulated.String(), "```tool_code") {
					return // buffering — will be rescued after stream completes
				}
				// Filter narration ("I'll search for...")
				if strings.HasPrefix(strings.TrimSpace(accumulated.String()), "I'll ") && accumulated.Len() < 200 && strings.Contains(chunk.Content, ".") {
					// Agent is narrating what it will do — skip until it actually does it
					return
				}

				onEvent(TextDelta(chunk.Content))
			}
			if chunk.Thinking != "" {
				onEvent(ThinkingDelta(chunk.Thinking))
			}
		})
		// Per-turn token telemetry — visible in structured logs for cost/cache dashboards.
		if llmResp != nil && llmResp.Usage != nil {
			u := llmResp.Usage
			slog.Info("agent.loop.tokens",
				"agent", ag.AgentKey,
				"session", req.SessionID,
				"iter", iter,
				"model", model,
				"prompt_tokens", u.PromptTokens,
				"completion_tokens", u.CompletionTokens,
				"total_tokens", u.TotalTokens,
				"cache_creation_tokens", u.CacheCreationTokens,
				"cache_read_tokens", u.CacheReadTokens,
				"tools_on_wire", len(toolDefs),
			)
		}

		if err != nil {
			slog.Warn("agent.loop.llm_error", "agent", ag.ID, "iter", iter, "model", model, "error", err)

			// Classify the error to decide recovery strategy
			reason := providers.ClassifyError(err.Error())
			recovered := false

			// Strategy 1: Auth/rate-limit → rotate to next API key (same model, different key).
			// This is the "never die on 401" fix.
			if !recovered && l.providerReg != nil &&
				(reason == providers.FailoverAuth || reason == providers.FailoverRateLimit || reason == providers.FailoverBilling) {
				if rotated, rotatedProvider := l.providerReg.RotateKey(provider, model); rotated {
					slog.Info("agent.loop.key_rotated", "agent", ag.ID, "reason", reason, "model", model)
					provider = rotatedProvider
					llmResp, err = provider.ChatStream(ctx, chatReq, func(chunk providers.StreamChunk) {
						if chunk.Content != "" {
							onEvent(TextDelta(chunk.Content))
						}
					})
					if err == nil {
						slog.Info("agent.loop.key_rotation_success", "agent", ag.ID)
						recovered = true
					} else {
						slog.Warn("agent.loop.key_rotation_failed", "agent", ag.ID, "error", err)
					}
				}
			}

			// Strategy 2: Model fallback — try alternative models.
			if !recovered {
				fallbackModels := []string{}
				if ag.FallbackModel != "" && ag.FallbackModel != model {
					fallbackModels = append(fallbackModels, ag.FallbackModel)
				}
				for _, fb := range l.getConfiguredModels() {
					if fb != model {
						fallbackModels = append(fallbackModels, fb)
					}
				}
				for _, fbModel := range fallbackModels {
					slog.Info("agent.loop.auto_fallback", "agent", ag.ID, "from", model, "to", fbModel, "reason", reason)
					chatReq.Model = fbModel
					llmResp, err = provider.ChatStream(ctx, chatReq, func(chunk providers.StreamChunk) {
						if chunk.Content != "" {
							onEvent(TextDelta(chunk.Content))
						}
					})
					if err == nil {
						model = fbModel
						recovered = true
						usedFallback = true
						slog.Info("agent.loop.fallback_success", "agent", ag.ID, "model", fbModel)
						break
					}
				}
			}

			if !recovered {
				slog.Error("agent.loop.all_models_failed", "agent", ag.ID, "last_reason", reason)
				onEvent(ErrorEvent("All LLM providers failed (" + string(reason) + "). Check your provider keys and configuration."))
				return result, err
			}
		} // end if err != nil

		// Track usage
		if llmResp.Usage != nil {
			result.InputTokens += llmResp.Usage.PromptTokens
			result.OutputTokens += llmResp.Usage.CompletionTokens
		}

		// Rescue narrated tool calls — LLMs sometimes output tool calls as
		// ```tool_code``` blocks or XML instead of proper function_call format.
		if len(llmResp.ToolCalls) == 0 && llmResp.Content != "" {
			rescued, cleaned := extractNarratedToolCalls(llmResp.Content, allowedTools)
			if len(rescued) > 0 {
				llmResp.ToolCalls = rescued
				llmResp.Content = cleaned
				slog.Info("agent.loop.tool_rescue", "agent", ag.ID, "rescued", len(rescued),
					"tools", func() string {
						var names []string
						for _, tc := range rescued {
							names = append(names, tc.Name)
						}
						return strings.Join(names, ",")
					}())
			}
		}

		// If no tool calls, we're done — this is the final response
		if len(llmResp.ToolCalls) == 0 {
			consecutiveToolIters = 0 // reset — agent produced text
			// Pattern 4: Reflection — self-review before sending
			// On first iteration with substantial output, do a quick quality check
			if iter == 0 && len(llmResp.Content) > 200 && ag.AutoCompact {
				// Ask LLM to verify its own response (cheap, fast)
				checkResp, checkErr := provider.Chat(ctx, providers.ChatRequest{
					Model: model,
					Messages: []providers.Message{
						{Role: "system", Content: "You are a quality checker. Review this response for accuracy and completeness. If it's good, reply ONLY with 'OK'. If it needs improvement, reply with the improved version."},
						{Role: "user", Content: llmResp.Content},
					},
					Options: map[string]any{"temperature": 0.1, "max_tokens": 500},
				})
				if checkErr == nil && checkResp.Content != "" {
					trimmed := strings.TrimSpace(checkResp.Content)
					if trimmed != "OK" && trimmed != "Ok" && trimmed != "ok" && len(trimmed) > 20 {
						llmResp.Content = checkResp.Content
						slog.Info("agent.loop.reflection", "agent", ag.ID, "improved", true)
					}
				}
			}

			lastLLMContent = llmResp.Content
			result.Content = stripHallucinatedToolCalls(llmResp.Content)
			// Visible-reply enforcement: if the model finished without tool
			// calls but produced no text, and tools were used earlier this
			// turn, force one more synthesis pass before giving up.
			// A tool-only turn that silently returns a blank bubble is the
			// worst UX failure mode.  Only retry once (iter check prevents
			// infinite loops); on the second empty we fall through to the
			// apology substitution below.
			if result.Content == "" && len(llmResp.ToolCalls) == 0 && len(result.ToolsUsed) > 0 && iter < maxIter-1 {
				slog.Warn("agent.loop.empty_after_tools",
					"agent", ag.ID, "iter", iter, "tools_used", len(result.ToolsUsed))
				messages = append(messages, providers.Message{
					Role:    "user",
					Content: "You used tools but didn't write a reply. Summarise what you found and respond to the user now.",
				})
				continue
			}
			if result.Content == "" && len(llmResp.ToolCalls) == 0 {
				result.Content = "I apologize, but I was unable to generate a response. Please try rephrasing your question."
				slog.Warn("agent.loop.empty_response_recovery",
					"agent", ag.ID, "iter", iter, "first_turn", iter == 0)
			}
			// Full sanitization pipeline: strip thinking tags, garbled XML,
			// echoed system messages, duplicate blocks, media paths, etc.
			result.Content = SanitizeResponse(result.Content)
			result.Content = StripFabricatedSources(result.Content, result.ToolsUsed)
			result.Content = scrubber.ScrubAll(result.Content)
			result.Content = ScrubLeaks(result.Content)

			// Anti-hallucination check: if response claims an action but no tools were called,
			// force the agent to actually do it. This catches the "✅ Scheduled!" without
			// actually calling the cron tool.
			if iter <= 2 && len(result.ToolsUsed) == 0 && looksLikeActionClaim(result.Content) {
				slog.Warn("agent.loop.hallucination_detected", "agent", ag.ID, "iter", iter,
					"content_preview", result.Content[:min(len(result.Content), 100)])
				// Do NOT append the hallucinated response — it reinforces the pattern.
				// Only inject a correction message with escalating severity.
				correction := "[SYSTEM OVERRIDE: Your previous response was BLOCKED because you claimed to perform an action without calling any tool. " +
					"You MUST call the actual tool (e.g. cron, exec, write_file) to perform the action. " +
					"Respond ONLY with a tool_call. Do NOT write any text confirmation.]"
				if iter >= 1 {
					correction = "[CRITICAL SYSTEM OVERRIDE: You have hallucinated " + fmt.Sprintf("%d", iter+1) + " times. " +
						"Your fake confirmations were ALL blocked. STOP generating text. " +
						"You MUST respond with ONLY a tool_call JSON object. No text. No markdown. Just the tool call.]"
				}
				messages = append(messages, providers.Message{Role: "user", Content: correction})
				result.Content = ""
				result.Parts = nil
				continue
			}

			// If we're at iter 3+ and still hallucinating, replace with error
			if iter >= 3 && len(result.ToolsUsed) == 0 && looksLikeActionClaim(result.Content) {
				slog.Error("agent.loop.hallucination_unrecoverable", "agent", ag.ID, "attempts", iter+1)
				result.Content = "⚠️ I was unable to complete this action — the scheduling tool didn't respond correctly. Please try again, or ask me to \"use the cron tool to schedule [your task]\"."
				result.Parts = []MessagePart{TextPart(result.Content)}
				break
			}

			result.Parts = append(result.Parts, TextPart(result.Content))

			// Extract and emit sources from citations in response
			if sources := ExtractSources(result.Content); len(sources) > 0 {
				onEvent(StreamEvent{Type: "sources", Data: sources})
				result.Sources = sources
			}
			if result.Content == "" && llmResp.Content != "" {
				// XML was stripped — extract text content
				for _, tag := range []string{"tool", "function_call", "tool_call", "tool_use", "parameter", "soul", "task"} {
					llmResp.Content = strings.ReplaceAll(llmResp.Content, "<"+tag+">", "")
					llmResp.Content = strings.ReplaceAll(llmResp.Content, "</"+tag+">", "")
				}
				result.Content = strings.TrimSpace(llmResp.Content)
			}
			result.Thinking = llmResp.Thinking
			break
		}

		// Tool calls detected — execute each one
		// If the model streamed text before deciding to call tools (e.g. "Hello! Did you
		// know…" then web_search), that preamble is narration not a final answer.
		// Reset the client's accumulated text so the real answer replaces it.
		if streamedChars > 0 {
			onEvent(StreamReset())
		}
		// Append assistant message with tool calls to context
		assistantMsg := providers.Message{
			Role:      "assistant",
			Content:   llmResp.Content,
			ToolCalls: llmResp.ToolCalls,
		}
		messages = append(messages, assistantMsg)

		consecutiveToolIters++ // track tool-only iterations

		// Parallel pre-execution: run read-only tools concurrently if multiple calls
		parallelResults := l.runParallelTools(ctx, toolCtx, req, llmResp.ToolCalls, allowedTools)

		for _, tc := range llmResp.ToolCalls {
			if ctx.Err() != nil {
				break
			}
			// Refuse tool calls with truncated / invalid JSON arguments.
			// Executing a valid-name-but-empty-args call is actively
			// dangerous for write_file / exec / rm-style tools. Tell
			// the model what happened and let it retry — better than
			// a silent no-op or an unintended side effect.
			if tc.ArgsParseError != "" {
				slog.Warn("agent.loop.tool_args_rejected",
					"tool", tc.Name, "reason", tc.ArgsParseError)
				messages = append(messages, providers.Message{
					Role: "tool",
					Content: "⛔ Tool call rejected — arguments JSON was invalid or truncated mid-stream. " +
						"Repeat the tool call with complete, well-formed JSON arguments.",
					ToolCallID: tc.ID,
				})
				loopGuard.RecordError()
				continue
			}
			// Skip tools already executed in parallel
			if pr, ok := parallelResults[tc.ID]; ok {
				loopGuard.RecordCall(tc.Name, tc.Arguments)
				if !pr.IsError {
					loopGuard.RecordToolSuccess(tc.Name)
				} else {
					loopGuard.RecordToolError(tc.Name)
				}
				result.ToolsUsed = append(result.ToolsUsed, tc.Name)
				content := TruncateToolResult(pr.ForLLM, tools.MaxToolOutput)
				onEvent(ToolStart(tc.Name))
				onEvent(ToolResult(tc.Name, truncateForEvent(content)))
				messages = append(messages, providers.Message{Role: "tool", Content: content, ToolCallID: tc.ID})
				slog.Info("agent.loop.parallel_done", "tool", tc.Name)
				continue
			}

			onEvent(ToolStart(tc.Name))
			onEvent(PartEvent(ToolCallPart(tc.Name, tc.ID, tc.Arguments)))
			toolStartTime := time.Now()
			slog.Info("agent.loop.tool_call", "agent", ag.ID, "tool", tc.Name, "iter", iter)

			// LoopGuard: record call and check for loops
			argsHash := loopGuard.RecordCall(tc.Name, tc.Arguments)
			if det := loopGuard.DetectSameArgs(tc.Name, argsHash); det.Level != DetectionNone {
				if det.Level == DetectionCritical {
					slog.Error("agent.loop.loop_detected", "tool", tc.Name, "msg", det.Message)
					result.Content = det.Message
					break
				}
				// Warning: inject into messages so LLM sees it
				messages = append(messages, providers.Message{Role: "user", Content: det.Message})
			}

			// ShellSecurity: check exec/bash commands
			if tc.Name == "exec" || tc.Name == "bash" {
				if cmd, ok := tc.Arguments["command"].(string); ok {
					check := shellSec.CheckCommand(cmd)
					if !check.Allowed && !check.AskUser {
						// Blocked by deny group
						slog.Warn("agent.loop.shell_blocked", "cmd", cmd, "reason", check.Reason, "group", check.Group)
						messages = append(messages, providers.Message{
							Role: "tool", Content: "⛔ " + check.Reason, ToolCallID: tc.ID,
						})
						loopGuard.RecordError()
						continue
					}
					if check.AskUser {
						// Create approval record and emit event for frontend.
						//
						// Uses a dedicated tool_approvals table rather than
						// the plan-centric `approvals` table — that one
						// requires a matching plans row + plan_nodes row
						// (via FKs approvals_plan_id_fkey and
						// approvals_node_id_fkey) and we don't have a plan
						// here; an ad-hoc shell command isn't a plan node.
						//
						// The old SQL targeted a legacy schema that no
						// longer exists (tenant_id/agent_id/tool_name/
						// tool_args/status) — INSERT silently failed,
						// approval_id was empty, and the poll timed out
						// after 60s even on benign commands like `curl`.
						//
						// The table is created lazily the first time we
						// reach this branch; CREATE TABLE IF NOT EXISTS
						// keeps it off the migrations path while still
						// persisting approvals durably.
						approvalID := ""
						if l.agentStore != nil && l.agentStore.Pool() != nil {
							pool := l.agentStore.Pool()
							_, _ = pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS tool_approvals (
  id           UUID        PRIMARY KEY DEFAULT uuid_generate_v7(),
  tenant_id   TEXT         NOT NULL,
  agent_id    TEXT         NOT NULL,
  tool_name   TEXT         NOT NULL,
  tool_args   JSONB        NOT NULL DEFAULT '{}'::jsonb,
  reason      TEXT         NOT NULL DEFAULT '',
  status      TEXT         NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','expired')),
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
  decided_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS tool_approvals_pending ON tool_approvals(agent_id, status) WHERE status = 'pending';`)
							args, _ := json.Marshal(tc.Arguments)
							err := pool.QueryRow(ctx,
								`INSERT INTO tool_approvals (tenant_id, agent_id, tool_name, tool_args, reason)
								 VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
								l.tenantID, ag.ID, tc.Name, string(args), check.Reason).Scan(&approvalID)
							if err != nil {
								slog.Warn("agent.loop.approval_insert_failed", "error", err)
							}
						}
						onEvent(StreamEvent{Type: "tool_approval", Data: map[string]any{
							"approval_id": approvalID, "tool": tc.Name, "command": cmd, "reason": check.Reason,
						}})
						// Poll for decision (max 60s)
						approved := false
						for i := 0; i < 30; i++ {
							time.Sleep(2 * time.Second)
							var status string
							if l.agentStore != nil && l.agentStore.Pool() != nil && approvalID != "" {
								l.agentStore.Pool().QueryRow(ctx,
									`SELECT status FROM tool_approvals WHERE id = $1`, approvalID).Scan(&status)
							}
							if status == "approved" {
								approved = true
								break
							}
							if status == "rejected" {
								break
							}
							if ctx.Err() != nil {
								break
							}
						}
						if !approved {
							messages = append(messages, providers.Message{
								Role: "tool", Content: "⛔ Command rejected by user or timed out.", ToolCallID: tc.ID,
							})
							continue
						}
						// Approved — add to allowlist and proceed
						shellSec.ApproveCommand(cmd)
						slog.Info("agent.loop.shell_approved", "cmd", cmd)
					}
				}
			}

			// Permission check: verify agent role allows this tool
			if ag.Role != nil {
				agentPerms := permissions.DefaultPermissions(permissions.Role(*ag.Role))
				allowed := permissions.FilterTools(permissions.Role(*ag.Role), agentPerms, []string{tc.Name})
				if len(allowed) == 0 {
					slog.Warn("agent.loop.permission_denied", "agent", ag.ID, "role", *ag.Role, "tool", tc.Name)
					messages = append(messages, providers.Message{
						Role: "tool", Content: "⛔ Permission denied: your role does not allow using " + tc.Name, ToolCallID: tc.ID,
					})
					continue
				}
			}

			// Execute tool with self-healing retry
			var toolResult *tools.Result

			// ENFORCEMENT: Skip tools not in allowedTools (gated by intent)
			if !allowedTools[tc.Name] {
				slog.Warn("agent.loop.tool_blocked", "agent", ag.ID, "tool", tc.Name, "allowed", len(allowedTools))
				toolResult = tools.ErrorResult(fmt.Sprintf("Tool '%s' is not available for this query type. Answer from your knowledge instead.", tc.Name))
			} else if tc.Name == "web_search" && webSearchCalls >= 2 && chatIntent == ChatIntentResearch {
				// Hard budget: max 2 web_search calls for Research intent
				slog.Warn("agent.loop.web_search_budget", "agent", ag.ID, "calls", webSearchCalls)
				toolResult = tools.ErrorResult("Web search budget exhausted (2/2). Summarize your findings and answer from the search results you have.")
			} else if tc.Name == "web_fetch" && webFetchCalls >= 1 && chatIntent == ChatIntentResearch {
				// Hard budget: max 1 web_fetch call for Research intent (prevents loops)
				slog.Warn("agent.loop.web_fetch_budget", "agent", ag.ID, "calls", webFetchCalls)
				toolResult = tools.ErrorResult("Web fetch budget exhausted (1/1). Use the content you already fetched to answer.")
			} else if loopGuard.IsToolCircuitBroken(tc.Name) {
				toolResult = tools.ErrorResult(fmt.Sprintf("⛔ %s circuit broken — failed 2x consecutively. Try a different approach.", tc.Name))
			} else {
				// Fire pre-tool hook
				if l.pluginMgr != nil {
					l.pluginMgr.FireHook(ctx, plugin.HookPreToolCall, map[string]any{
						"tool_name": tc.Name, "args": tc.Arguments, "agent_id": ag.ID,
					})
				}
				for retry := 0; retry < 3; retry++ {
					toolResult = l.executeTool(toolCtx, req, tc.Name, tc.Arguments)
					// Collect media files from tool results
					for _, m := range toolResult.Media {
						result.Media = append(result.Media, MediaResult{Path: m.Path, ContentType: m.MimeType})
					}
					if !toolResult.IsError {
						loopGuard.RecordToolSuccess(tc.Name)
						break
					}
					loopGuard.RecordToolError(tc.Name)
					if retry < 2 {
						slog.Warn("agent.loop.tool_retry", "tool", tc.Name, "retry", retry+1, "error", toolResult.ForLLM[:min(len(toolResult.ForLLM), 100)])
						toolResult.ForLLM = "⚠️ Tool failed: " + toolResult.ForLLM + "\nPlease fix the arguments and try again."
					}
				}
				// Fire post-tool hook
				if l.pluginMgr != nil {
					l.pluginMgr.FireHook(ctx, plugin.HookPostToolCall, map[string]any{
						"tool_name": tc.Name, "args": tc.Arguments, "result": toolResult.ForLLM,
						"is_error": toolResult.IsError, "agent_id": ag.ID,
					})
				}

				// Audit: log tool execution
				if l.auditFn != nil {
					l.auditFn(ag.AgentKey, tc.Name, req.SessionID, toolResult.IsError)
				}
			}

			// LoopGuard: record result and check for same-result loops
			loopGuard.RecordResult(tc.Name, argsHash, toolResult.ForLLM)
			loopGuard.RecordMutation(tc.Name)

			// Publish tool.called event
			if l.Events != nil {
				l.Events.Publish(DomainEvent{
					Type: EvtToolCalled, AgentID: ag.ID,
					Data: map[string]any{"tool": tc.Name, "session": req.SessionID, "error": toolResult.IsError},
				})
			}

			// Send review request to Prime for high-risk tool calls
			if l.SupervisorBus != nil && l.PrimeID != "" && ag.ID != l.PrimeID {
				if isMutatingTool(tc.Name) && !toolResult.IsError {
					go func(toolName, result string) {
						l.SupervisorBus.Send(context.Background(), supervisorpkg.Message{
							From:    ag.ID,
							To:      l.PrimeID,
							Intent:  supervisorpkg.IntentReviewRequest,
							Content: fmt.Sprintf("Tool %s executed with side effects. Please verify.", toolName),
							Risk:    supervisorpkg.RiskMedium,
							Context: map[string]any{
								"tool":       toolName,
								"session_id": req.SessionID,
								"output":     result[:min2(len(result), 500)],
							},
						})
					}(tc.Name, toolResult.ForLLM)
				}
			}
			if det := loopGuard.DetectSameResult(tc.Name, hashResult(toolResult.ForLLM)); det.Level == DetectionCritical {
				slog.Error("agent.loop.same_result_loop", "tool", tc.Name, "msg", det.Message)
				result.Content = det.Message
				break
			}
			if det := loopGuard.DetectReadOnlyStreak(); det.Level != DetectionNone {
				if det.Level == DetectionCritical {
					result.Content = det.Message
					break
				}
				messages = append(messages, providers.Message{Role: "user", Content: det.Message})
			}

			result.ToolsUsed = append(result.ToolsUsed, tc.Name)

			// Track web_search calls for budget
			if tc.Name == "web_search" {
				webSearchCalls++
				slog.Info("agent.loop.web_search_call", "agent", ag.ID, "call", webSearchCalls)
			}
			// Track web_fetch calls for budget
			if tc.Name == "web_fetch" {
				webFetchCalls++
				slog.Info("agent.loop.web_fetch_call", "agent", ag.ID, "call", webFetchCalls)
			}

			// SecretScrubber: scrub tool output before adding to context
			content := scrubber.ScrubAll(toolResult.ForLLM)
			content = ScrubLeaks(content)
			// PII redaction on tool output — scraped pages, emails,
			// database rows often carry real PII that the tenant may
			// not want leaving the workspace. Applied after credential
			// scrubbing so token shapes don't collide with PII regexes.
			if l.PIIRedactor != nil {
				content = l.PIIRedactor.Redact(content)
			}

			// Truncate long results for context, preserving head+tail around error/summary keywords
			content = TruncateToolResult(content, tools.MaxToolOutput)

			toolDur := int(time.Since(toolStartTime).Milliseconds())
			onEvent(ToolResult(tc.Name, truncateForEvent(content)))
			onEvent(PartEvent(ToolResultPart(tc.Name, tc.ID, truncateForEvent(content), toolDur)))
			result.Parts = append(result.Parts, ToolCallPart(tc.Name, tc.ID, tc.Arguments), ToolResultPart(tc.Name, tc.ID, truncateForEvent(content), toolDur))

			// Rich widget emission. Tools that want a card alongside
			// their text reply set Result.Widget (one) or Result.Widgets
			// (many — e.g. browse_and_act emits a browser_step card
			// per step). Each becomes a streaming widget event AND an
			// archived WidgetPart so the cards show up during
			// streaming and in the final message record.
			if toolResult.Widget != nil {
				w := toolResult.Widget
				onEvent(WidgetEvent(map[string]any{"type": w.Type, "data": w.Data}))
				onEvent(PartEvent(WidgetPart(w.Type, w.Data)))
				result.Parts = append(result.Parts, WidgetPart(w.Type, w.Data))
			}
			for _, w := range toolResult.Widgets {
				onEvent(WidgetEvent(map[string]any{"type": w.Type, "data": w.Data}))
				onEvent(PartEvent(WidgetPart(w.Type, w.Data)))
				result.Parts = append(result.Parts, WidgetPart(w.Type, w.Data))
			}

			// Append tool result to context for next LLM call
			messages = append(messages, providers.Message{
				Role:       "tool",
				Content:    content,
				ToolCallID: tc.ID,
			})
		}

		// Qorven-style total tool budget: after 15 total tool calls, force summarize
		if len(result.ToolsUsed) >= 15 {
			slog.Warn("agent.loop.tool_budget", "agent", ag.ID, "total", len(result.ToolsUsed))
			messages = append(messages, providers.Message{
				Role:    "user",
				Content: "[System] Tool budget reached. Summarize your findings and respond to the user now.",
			})
			// One more LLM call to summarize, then loop exits (no tool calls in response)
		}

		// Loop continues — LLM will see tool results and decide next action
	}

	// Self-healing: if tools were used but no text content, try fallback models
	if result.Content == "" && len(result.ToolsUsed) > 0 {
		slog.Warn("agent.loop.empty_content", "agent", ag.ID, "model", model, "tools_used", len(result.ToolsUsed))

		// Convert tool messages to user message (some models can't handle tool role)
		var toolSummary strings.Builder
		toolSummary.WriteString("Here are the search results. Summarize them in a structured answer with headers and bullet points:\n\n")

		cleanMessages := make([]providers.Message, 0, len(messages))
		for _, m := range messages {
			if m.Role == "tool" {
				toolSummary.WriteString(m.Content + "\n\n")
			} else if m.Role == "assistant" && len(m.ToolCalls) > 0 {
				continue // skip tool call messages
			} else {
				cleanMessages = append(cleanMessages, m)
			}
		}
		cleanMessages = append(cleanMessages, providers.Message{Role: "user", Content: toolSummary.String()})
		fallbackResp, err := provider.Chat(ctx, providers.ChatRequest{
			Model: model, Messages: cleanMessages,
			Options: map[string]any{"temperature": 0.3, "max_tokens": 2000},
		})
		if err == nil && fallbackResp.Content != "" {
			result.Content = fallbackResp.Content
			slog.Info("agent.loop.self_healed", "agent", ag.ID, "method", "retry_same_model")
		}

		// If still empty, try agent's fallback model first, then other available models
		if result.Content == "" {
			var fallbackModels []string
			if ag.FallbackModel != "" {
				fallbackModels = append(fallbackModels, ag.FallbackModel)
			}
			// Add common fallbacks
			for _, fb := range l.getConfiguredModels() {
				if fb != model && fb != ag.FallbackModel {
					fallbackModels = append(fallbackModels, fb)
				}
			}
			for _, fbModel := range fallbackModels {
				if fbModel == model {
					continue
				} // skip the one that already failed
				slog.Info("agent.loop.model_fallback", "agent", ag.ID, "from", model, "to", fbModel)
				fbResp, err := provider.Chat(ctx, providers.ChatRequest{
					Model: fbModel, Messages: messages,
					Options: map[string]any{"temperature": 0.3, "max_tokens": 2000},
				})
				if err == nil && fbResp.Content != "" {
					result.Content = fbResp.Content
					slog.Info("agent.loop.self_healed", "agent", ag.ID, "method", "model_fallback", "model", fbModel)
					break
				}
			}
		}

		// If ALL models failed, report back to user
		if result.Content == "" {
			result.Content = fmt.Sprintf("⚠️ I completed the task using %d tools but couldn't generate a summary. The tools returned data but the response was empty. Please try again or ask me to retry.", len(result.ToolsUsed))
			slog.Error("agent.loop.all_models_failed", "agent", ag.ID, "tools_used", len(result.ToolsUsed))
		}
	}

	// 9. Save messages to session
	l.saveToSession(ctx, req, result)

	// Gap D fix: Prime Coder Project Memory
	// After a coding session turn, extract key findings (bugs fixed, conventions discovered,
	// architectural decisions) into ScopeTask memory keyed to the project session.
	// Any agent or future session on the same project gets this injected automatically.
	if strings.HasPrefix(req.SessionID, "code-") && l.HierarchyMem != nil &&
		result.Content != "" && len(result.ToolsUsed) > 0 {
		go func() {
			bgCtx := context.Background()
			// Only save substantive responses (not just "done" or one-liners)
			if len(result.Content) < 100 {
				return
			}
			// Derive project path for context
			projectPath := ""
			if l.projectReg != nil {
				for _, p := range l.projectReg.List() {
					if p.SessionID == req.SessionID {
						projectPath = p.Path
						break
					}
				}
			}
			if projectPath == "" {
				return
			}

			// Build a compact learning entry: what the agent did + key outcome
			tools := strings.Join(result.ToolsUsed, ", ")
			entry := fmt.Sprintf("Project: %s | Tools used: %s | Task: %s | Outcome: %s",
				projectPath,
				tools,
				truncateForProjectMem(req.UserMessage, 150),
				truncateForProjectMem(result.Content, 300),
			)
			if _, err := l.HierarchyMem.SaveTask(bgCtx, req.SessionID, ag.ID, entry, "prime_coder"); err != nil {
				slog.Debug("prime_coder.project_mem.failed", "session", req.SessionID, "error", err)
			}
		}()
	}

	// 11. Background tasks: title, tags, follow-ups
	// Run BEFORE DoneEvent so SSE connection is still open
	isFirstMsg := len(history) == 0
	l.RunBackgroundTasks(ctx, provider, ag.ID, req.SessionID, model,
		req.UserMessage, result.Content, isFirstMsg, onEvent)

	doneSent = true
	onEvent(DoneEvent())

	// 10. Send heartbeat to Prime (supervisor protocol)
	if l.SupervisorBus != nil && l.PrimeID != "" && ag.ID != l.PrimeID {
		go func() {
			l.SupervisorBus.Send(context.Background(), supervisorpkg.Message{
				From:    ag.ID,
				To:      l.PrimeID,
				Intent:  supervisorpkg.IntentHeartbeat,
				Content: fmt.Sprintf("Run complete: %d tokens, %d iterations, %d tools used", result.InputTokens+result.OutputTokens, result.Iterations, len(result.ToolsUsed)),
				Context: map[string]any{
					"input_tokens":  result.InputTokens,
					"output_tokens": result.OutputTokens,
					"iterations":    result.Iterations,
					"tools_used":    result.ToolsUsed,
					"duration_ms":   time.Since(start).Milliseconds(),
					"session_id":    req.SessionID,
					"has_errors":    result.Content == "",
				},
			})
		}()
	}

	// 11. Extract memories (background, don't block response)
	go l.extractMemories(context.Background(), ag.ID, req.UserMessage, result.Content, req.SessionID)

	// 11b. Self-improving skills: analyze session for learnable patterns
	if l.skillLearner != nil && result.Content != "" && len(result.ToolsUsed) >= 3 {
		go func() {
			traces := make([]skills.ToolTrace, 0, len(result.ToolsUsed))
			for _, t := range result.ToolsUsed {
				traces = append(traces, skills.ToolTrace{Tool: t, Success: true, Timestamp: time.Now()})
			}
			outcome := skills.SessionOutcome{
				SessionID:   req.SessionID,
				AgentID:     ag.ID,
				UserQuery:   req.UserMessage,
				ToolTraces:  traces,
				FinalAnswer: result.Content,
				Success:     true,
				Duration:    time.Since(start),
			}
			if err := l.skillLearner.AnalyzeAndLearn(context.Background(), outcome); err != nil {
				slog.Warn("skill.learn.failed", "agent", ag.ID, "error", err)
			}
		}()
	}

	// 11c. Fire plugin post-run hooks
	if l.pluginMgr != nil {
		go l.pluginMgr.FireHook(context.Background(), plugin.HookPostLLMCall, map[string]any{
			"agent_id":    ag.ID,
			"session_id":  req.SessionID,
			"tokens":      result.InputTokens + result.OutputTokens,
			"tools_used":  result.ToolsUsed,
			"duration_ms": time.Since(start).Milliseconds(),
		})
	}

	// 12. Flush memories for long conversations (Qorven pattern)
	if len(history) > 10 && l.memStore != nil {
		go func() {
			provider := l.providerReg.Default()
			if provider == nil {
				return
			}
			curated := memory.NewCuratedStore("")
			err := memory.FlushMemories(
				context.Background(), provider, "",
				history, curated, memory.DefaultFlushConfig(),
			)
			if err != nil {
				slog.Warn("memory.flush.failed", "session", req.SessionID, "error", err)
			} else {
				slog.Info("memory.flush.complete", "session", req.SessionID, "history_len", len(history))
			}
		}()
	}

	// 13. Populate working memory with this turn's events
	if l.WorkingMem != nil {
		l.WorkingMem.Emit(memory.EventUserMessage, fmt.Sprintf("User: %s → Agent: %s", truncateStr(req.UserMessage, 100), truncateStr(result.Content, 100))).
			Importance(0.5).Record()
		for _, tool := range result.ToolsUsed {
			l.WorkingMem.Emit(memory.EventToolCall, tool).Importance(0.3).Record()
		}
		if result.Content == "" {
			l.WorkingMem.Emit(memory.EventType("error"), "Empty response").Importance(0.8).Record()
		}
	}

	// Pattern 9: Learning — detect if user is correcting a previous response
	if l.agentStore != nil && len(history) > 2 {
		lastAssistant := ""
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "assistant" {
				lastAssistant = history[i].Content
				break
			}
		}
		if lastAssistant != "" && isCorrection(req.UserMessage) {
			l.agentStore.Pool().Exec(context.Background(),
				`INSERT INTO feedback (agent_id, session_id, agent_response, correction, feedback_type) VALUES ($1, $2, $3, $4, 'correction')`,
				ag.ID, req.SessionID, lastAssistant[:min(len(lastAssistant), 500)], req.UserMessage[:min(len(req.UserMessage), 500)])
		}
	}

	dur := time.Since(start)
	slog.Info("agent.loop.complete",
		"agent", ag.ID, "session", req.SessionID,
		"iterations", result.Iterations, "tools_used", len(result.ToolsUsed),
		"input_tokens", result.InputTokens, "output_tokens", result.OutputTokens,
		"duration_ms", dur.Milliseconds())

	// Learning Loop — auto-retrospective after every task
	if l.LearningLoop != nil && result.Content != "" {
		go l.LearningLoop.RunAfterTask(context.Background(), ag.ID, l.tenantID, req.UserMessage, result.Content, result.ToolsUsed)
	}

	// Track token usage for budget enforcement
	if l.agentStore != nil {
		l.agentStore.TrackUsage(context.Background(), ag.ID, result.InputTokens, result.OutputTokens)

		// Log cost event for billing dashboard
		if l.billingStore != nil {
			// Estimate cost based on model (rough pricing per 1K tokens)
			costPer1KInput := 0.003  // default ~$3/M input
			costPer1KOutput := 0.015 // default ~$15/M output
			cost := float64(result.InputTokens)/1000*costPer1KInput + float64(result.OutputTokens)/1000*costPer1KOutput
			l.billingStore.LogCostP(context.Background(), billing.CostEventParams{
				TenantID: l.tenantID, AgentID: ag.ID, SessionID: req.SessionID,
				Model: ag.Model, InputTokens: result.InputTokens, OutputTokens: result.OutputTokens,
				CostUSD: cost, TraceID: result.TraceID, LatencyMS: dur.Milliseconds(),
			})
		}
	}

	// Pattern 19: Track performance metrics
	if l.agentStore != nil {
		pool := l.agentStore.Pool()
		// Track response time
		pool.Exec(context.Background(),
			`INSERT INTO agent_metrics (agent_id, metric_type, value, metadata) VALUES ($1, 'response_time_ms', $2, $3)`,
			ag.ID, dur.Milliseconds(), fmt.Sprintf(`{"iterations":%d,"tools":%d}`, result.Iterations, len(result.ToolsUsed)))
		// Track tool success rate
		if len(result.ToolsUsed) > 0 {
			pool.Exec(context.Background(),
				`INSERT INTO agent_metrics (agent_id, metric_type, value) VALUES ($1, 'tools_used', $2)`,
				ag.ID, len(result.ToolsUsed))
		}
	}

	// OpenSpace: crystallize skills from successful multi-step runs (async)
	if l.Crystallizer != nil && result.Content != "" && len(result.ToolsUsed) >= 2 {
		go l.Crystallizer.MaybeExtract(context.Background(), req.AgentID, req.UserMessage, result.Content, result.ToolsUsed)
	}

	// Post-run: background summarization when history exceeds 60% of context window.
	// Prevents unbounded session growth. Runs async so it doesn't block the response.
	if l.sessionStore != nil && req.SessionID != "" && contextWindow > 0 {
		go func() {
			bgCtx := context.Background()
			sessHistory, err := l.sessionStore.GetHistory(bgCtx, req.SessionID)
			if err != nil || len(sessHistory) < 6 {
				return
			}
			// Convert session messages to provider messages for token estimation
			var provMsgs []providers.Message
			for _, m := range sessHistory {
				provMsgs = append(provMsgs, providers.Message{Role: m.Role, Content: m.Content})
			}
			tokens := EstimateHistoryTokens(provMsgs)
			threshold := int(float64(contextWindow) * 0.6)
			if tokens <= threshold {
				return
			}
			provider := l.resolveProvider(ag)
			if provider == nil {
				return
			}
			compacted := CompactWithLLM(bgCtx, provider, ag.Model, provMsgs, 4)
			if compacted != nil {
				var sessMsgs []session.Message
				for _, m := range compacted {
					sessMsgs = append(sessMsgs, session.Message{Role: m.Role, Content: m.Content})
				}
				l.sessionStore.SetHistory(bgCtx, req.SessionID, sessMsgs)
				slog.Info("post_run.summarized", "session", req.SessionID, "before", len(sessHistory), "after", len(sessMsgs))
			}
		}()
	}

	// Run post-hooks
	if l.Hooks != nil {
		l.Hooks.RunPost(ctx, &req, result, time.Since(start))
	}

	return result, nil
}
