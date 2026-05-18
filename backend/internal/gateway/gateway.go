// Copyright 2026 Qorven AI. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0 (ELv2)
// that can be found in the LICENSE file.

package gateway

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
	"github.com/qorvenai/qorven/internal/agent"
	"github.com/qorvenai/qorven/internal/bus"
	"github.com/qorvenai/qorven/internal/channels"
	"github.com/qorvenai/qorven/internal/config"
	"github.com/qorvenai/qorven/internal/engine"
	"github.com/qorvenai/qorven/internal/knowledgegraph"
	"github.com/qorvenai/qorven/internal/plugin"

	// Previously unregistered channels — fully implemented, now wired

	"github.com/qorvenai/qorven/internal/a2a"
	"github.com/qorvenai/qorven/internal/audit"
	"github.com/qorvenai/qorven/internal/auth"
	"github.com/qorvenai/qorven/internal/billing"
	"github.com/qorvenai/qorven/internal/calendar"
	"github.com/qorvenai/qorven/internal/connectors"
	cronpkg "github.com/qorvenai/qorven/internal/cron"
	"github.com/qorvenai/qorven/internal/drive"
	"github.com/qorvenai/qorven/internal/heartbeat"
	"github.com/qorvenai/qorven/internal/artificialanalysis"
	"github.com/qorvenai/qorven/internal/mail"
	"github.com/qorvenai/qorven/internal/mcp"
	"github.com/qorvenai/qorven/internal/memory"
	"github.com/qorvenai/qorven/internal/oauth"
	"github.com/qorvenai/qorven/internal/permissions"
	"github.com/qorvenai/qorven/internal/providers"
	"github.com/qorvenai/qorven/internal/rag"
	"github.com/qorvenai/qorven/internal/sandbox"
	supervisorpkg "github.com/qorvenai/qorven/internal/supervisor"
	systempkg "github.com/qorvenai/qorven/internal/system"
	"github.com/qorvenai/qorven/internal/vault"

	apicommands "github.com/qorvenai/qorven/internal/api/commands"
	apievents "github.com/qorvenai/qorven/internal/api/events"
	"github.com/qorvenai/qorven/internal/api/sessioncancel"
	"github.com/qorvenai/qorven/internal/agui"
	"github.com/qorvenai/qorven/internal/approvals"
	"github.com/qorvenai/qorven/internal/deployment"
	"github.com/qorvenai/qorven/internal/mediagen"
	"github.com/qorvenai/qorven/internal/notifications"
	orchestratorpkg "github.com/qorvenai/qorven/internal/orchestrator"
	"github.com/qorvenai/qorven/internal/plans"
	wasmregistry "github.com/qorvenai/qorven/internal/plugins/registry"
	"github.com/qorvenai/qorven/internal/plugins/wasm"
	"github.com/qorvenai/qorven/internal/presence"
	"github.com/qorvenai/qorven/internal/realtime"
	"github.com/qorvenai/qorven/internal/research"
	"github.com/qorvenai/qorven/internal/scenario"
	"github.com/qorvenai/qorven/internal/serviceaccounts"
	"github.com/qorvenai/qorven/internal/session"
	"github.com/qorvenai/qorven/internal/skills"
	"github.com/qorvenai/qorven/internal/souldesk"
	"github.com/qorvenai/qorven/internal/store"
	"github.com/qorvenai/qorven/internal/tasks"
	"github.com/qorvenai/qorven/internal/tools"
	"github.com/qorvenai/qorven/internal/voice"
	"github.com/qorvenai/qorven/internal/webintel"
	"github.com/qorvenai/qorven/internal/workflow"

	"github.com/qorvenai/qorven/internal/autonomy"
	"github.com/qorvenai/qorven/internal/briefing"
	"github.com/qorvenai/qorven/internal/daemon"
	"github.com/qorvenai/qorven/internal/dashboard"
	"github.com/qorvenai/qorven/internal/datasource"
	"github.com/qorvenai/qorven/internal/discussion"
	"github.com/qorvenai/qorven/internal/apps"
	"github.com/qorvenai/qorven/internal/inbound"
)

// embeddedMigrations holds the migrations FS injected by SetEmbeddedMigrations.
// The binary's main package embeds the migrations dir and calls that setter before gateway.New().
var embeddedMigrations fs.FS

// SetEmbeddedMigrations injects the embedded migrations FS so the gateway
// can run them on first boot without an external migrations/ directory.
// Call this from main() before gateway.New().
func SetEmbeddedMigrations(fsys fs.FS) { embeddedMigrations = fsys }

// buildInfo holds version metadata injected by the cmd package at startup.
var buildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

// SetBuildInfo injects version/commit/buildtime from the main binary.
// Call this from cmd/start.go before gateway.New().
func SetBuildInfo(version, commit, buildTime string) {
	buildInfo.Version = version
	buildInfo.Commit = commit
	buildInfo.BuildTime = buildTime
}

var changelogContent string

// SetChangelog injects the CHANGELOG.md content embedded by the binary.
func SetChangelog(s string) { changelogContent = s }

type Gateway struct {
	cfg              *config.Config
	router           chi.Router
	startTime        time.Time
	server           *http.Server
	shutdownOnce     sync.Once                    // Guards installShutdownHandler — Serve() may be called more than once in tests.
	sqlRegistry      *tools.SQLConnectionRegistry // registered user DBs for sql_query tool
	screenShare      *ScreenShareStore            // per-tenant latest user-shared screen frame
	db               *store.DB
	agents           *agent.Store
	sessions         *session.Store
	heartbeat        *agent.Heartbeat
	providerStore    *providers.Store
	providerReg      *providers.Registry
	scenarioHandlers *scenario.Handlers
	toolReg          *tools.Registry
	skillLoader      *skills.Loader
	skillStore       *skills.Store
	notifStore       *notifications.Store
	auditStore       *audit.Store
	discussionStore  *discussion.Store
	clusterer        *discussion.Clusterer
	billingStore     *billing.Store
	bundleStore      *agent.BundleStore
	customTools      *tools.CustomToolStore
	mcpClient        *mcp.Client
	agentLoop        *agent.Loop
	brain            *engine.Engine // Universal AI brain
	memStore         *memory.Store
	taskStore        *tasks.Store
	msgStore         *agent.MessageStore
	hbStore          *heartbeat.Store
	voiceMgr         *voice.Manager
	voicePipeline    *voice.VoicePipeline
	voiceStore       *voice.Store
	mediaMgr         *mediagen.Manager
	mediaStore       *mediagen.Store
	mentionRouter    *souldesk.MentionRouter
	soulDesk         *souldesk.SoulDesk
	rtHub            *realtime.Hub
	wfStore          *workflow.Store
	wfExecutor       *workflow.Executor
	chanMgr          *channels.Manager
	inbound          *inbound.Processor
	bindingStore     *channels.BindingStore
	pluginMgr        *plugin.Manager
	appMgr           *apps.AppManager
	a2aServer        *a2a.Server
	connReg          *connectors.Registry
	connKB           *connectors.KnowledgeStore
	connExec         *connectors.Executor
	vault            *vault.Vault
	oauthMgr         *oauth.Manager
	mailStore        *mail.Store
	mailRouter       *mail.Router
	driveStore       *drive.Store
	sandboxStore     *sandbox.Store
	calendarStore    *calendar.Store
	mcpManager       *mcp.Manager
	supervisor       *supervisorpkg.Supervisor
	codePipeline     *systempkg.Pipeline
	cronRunner       *cronpkg.Runner
	supervisorBus    *supervisorpkg.Bus
	researchEngine   *research.Engine
	msgBus           *bus.MessageBus
	kgStore          *knowledgegraph.Store
	projectReg       *tools.ProjectRegistry
	authSvc          *auth.AuthService
	dreamer          *memory.Dreamer // background memory consolidation

	briefingSched *briefing.Scheduler   // daily briefing cron (nil until DB available)
	dsScheduler   *datasource.Scheduler // connector data source cron (nil until DB available)

	tileStore *dashboard.TileStore     // pinned dashboard tiles (nil until DB available)
	snapStore *datasource.SnapshotStore // snapshot data for tile "data" field (nil until DB available)

	aaClient    *artificialanalysis.Client // nil when no API key configured
	aguiHandler *agui.Handler             // AG-UI protocol endpoint

	// Canonical event + command surfaces (Phase 1 protocol layer).
	events          *apievents.Emitter
	cmdServer       *apicommands.Server
	sessionCancels  *sessioncancel.Registry
	serviceAccounts *serviceaccounts.Store

	// Phase 2 stores + services.
	plans          *plans.Store
	approvals      *approvals.Store
	permissionGate *permissions.Gate
	orchestrator   *orchestratorpkg.Service

	// deployment-mode config (single_tenant / multi_tenant).
	// Consulted by authorize() and sweeper to branch strict rules
	// without flipping single-tenant behavior.
	deploymentConfig *deployment.Config

	// per-tenant sweeper supervisor. Populated only
	// in multi-tenant mode; single-tenant still uses a direct Sweeper.
	// Exposed for /v1/orchestrator/status admin tooling and tests.
	sweeperManager *orchestratorpkg.SweeperManager

	// tenant-scoped Wasm plugin registry + loader. Nil in
	// installs that haven't opted into runtime plugins. Upload and
	// list routes guard on nil.
	//
	// the Wasm host is ALSO a gateway-lifetime
	// singleton now. ensureProtocolSurfaces is idempotent and can
	// run multiple times (tests, config reloads); re-creating the
	// Host each call leaked compiled modules. The nil-check in
	// ensureProtocolSurfaces skips re-init once this field is set.
	wasmHost         *wasm.Host
	wasmPluginStore  *wasmregistry.Store
	wasmPluginLoader *wasmregistry.Loader

	// per-tenant quota (concurrency cap + token-bucket rate
	// limit). Installed via TenantQuotaMiddleware on the request
	// paths named by the ruling: /v1/commands + /v1/wasm-plugins.
	// Lazily built in ensureProtocolSurfaces. Nil-safe: a nil quota
	// disables the middleware entirely.
	tenantQuota *TenantQuota

	// Consumer state: tracks running task sessions for cancel, background goroutine lifecycle
	taskRunSessions sync.Map       // taskID → sessionKey (for cancel on task failure)
	bgWg            sync.WaitGroup // tracks background goroutines for graceful shutdown
	announceMu      sync.Map       // sessionKey → *sync.Mutex (serialize announce runs)
	redirectServer  *http.Server   // HTTP→HTTPS redirect server (tracked for graceful shutdown)

	// In-process PTY terminal sessions (browser ↔ shell over WebSocket).
	termStore *TerminalStore

	// Pending manual-mode delegations: sessionID → pendingDelegation.
	// Populated when a user triggers @soul task in manual delegation mode;
	// cleared when the user confirms ("yes") or cancels ("no"/"cancel").
	pendingDelegations sync.Map

	// Stop channel for the LLM stats background ticker (prevent goroutine leak on hot-reload).
	llmStatsStop chan struct{}

	// Sandbox app runner — manages Docker container lifecycle + reverse proxy.
	appRunner *sandbox.AppRunner

	// Multi-agent daemon: external agent registry, task queue, plan approval.
	daemonReg  *daemon.Registry
	daemonSvc  *daemon.Daemon  // lifecycle manager (workspace isolation + reapers)

	// Autonomous runtime (071)
	runtimeMgr      *agent.RuntimeManager  // persistent runtime manager (071)
	taskCoordinator *TaskCoordinator       // synthesis trigger (071)
}

func New(cfg *config.Config) (*Gateway, error) {
	r := chi.NewRouter()
	// Path alias: /api/v1/* → /v1/*, /api/auth/* → /auth/*,
	// /api/ws* → /ws*. Must be the first middleware so the rewrite
	// happens before RequestID, Logger, etc. see the real path.
	//
	// Why: the web UI in dev hits the backend via Next.js rewrites
	// under /api. When the same UI is served as a static export by
	// this binary there is no Next runtime to do that rewrite, so
	// we peel off the /api prefix here. The Core1 /api/memory,
	// /api/teams, /api/plugins routes (see gateway.go "Core 1 API
	// routes" Route("/api", …)) stay untouched because they don't
	// overlap with the aliased subpaths.
	r.Use(apiPrefixAlias)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(MaxBodySize(10 * 1024 * 1024)) // 10MB max request body
	r.Use(CORS(cfg.Server.AllowedOrigins))
	r.Use(securityHeaders)
	r.Use(NewIPRateLimit(200, 400).Middleware())
	r.Use(MetricsMiddleware)

	// Rate limiting: 200 req/s per IP (burst 400) — web UI loads many chunks per page
	rateLimiter := NewRateLimiter(600, time.Second)
	r.Use(RateLimitMiddleware(rateLimiter))
	// SEC-2: Auth token from env var (overrides config)
	if envToken := os.Getenv("AUTH_TOKEN"); envToken != "" {
		cfg.Auth.Token = envToken
	}

	// SEC-4: CORS — origins accepted, in priority order:
	//   1. config.toml  [server] allowed_origins
	//   2. CORS_ORIGINS env var  (comma-separated; handy for Docker/CI)
	//   3. default: reflect any Origin back ("*" with credentials is
	//      rejected by browsers, so we echo instead). This lets any
	//      OSS user run `next dev` on any port/IP without touching config.
	//      Operators who want stricter rules set allowed_origins in config.toml.
	var corsOrigins []string
	switch {
	case len(cfg.Server.AllowedOrigins) > 0:
		corsOrigins = cfg.Server.AllowedOrigins
	case os.Getenv("CORS_ORIGINS") != "":
		corsOrigins = strings.Split(os.Getenv("CORS_ORIGINS"), ",")
	default:
		// Special sentinel: our custom CORS() middleware (line ~251) already
		// handles echo-back when AllowedOrigins is empty. Use that path —
		// don't configure go-chi/cors at all in the open default case, since
		// go-chi/cors with ["*"] + AllowCredentials=true is invalid per spec.
		corsOrigins = nil
	}
	if len(corsOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   corsOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}
	// When corsOrigins is nil the CORS() middleware registered above at
	// r.Use(CORS(cfg.Server.AllowedOrigins)) already handles echo-back
	// for any origin (empty AllowedOrigins → allow all). No second
	// middleware needed.

	gw := &Gateway{
		cfg:         cfg,
		router:      r,
		startTime:   time.Now(),
		providerReg: providers.NewRegistry(),
		toolReg:     tools.NewRegistry(),
		mcpClient:   mcp.NewClient(),
		heartbeat:   agent.NewHeartbeat(),
		rtHub:       realtime.NewHub(),
		msgBus:      bus.New(),
		screenShare: NewScreenShareStore(),
		termStore:   newTerminalStore(),
		daemonReg:   daemon.New(), // fallback; daemonSvc overrides if repo root known
		// API server — historically the only listener. Now bound to
		// APIListen when the config uses the split schema. If the
		// config still uses the legacy Listen field, config.Load
		// has already copied it into APIListen for us.
		server: &http.Server{
			Addr: firstNonEmpty(cfg.Server.APIListen, cfg.Server.Listen, "127.0.0.1:4200"), Handler: r,
			ReadTimeout: 30 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
		},
	}

	// Connect DB
	if cfg.Database.DSN != "" {
		db, err := store.New(cfg.Database.DSN)
		if err != nil {
			slog.Warn("database not available", "error", err)
		} else {
			gw.db = db

			// Initialize auth service
			gw.authSvc = auth.NewAuthService(db.Pool)

			// Pre-create extensions (must run outside transaction)
			for _, ext := range []string{"pgcrypto", "vector", "uuid-ossp"} {
				db.Pool.Exec(context.Background(), fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS \"%s\"", ext))
			}
			// Create uuid_generate_v7 function if not exists.
			// Uses gen_random_bytes (pgcrypto) if available, otherwise builds random bytes
			// from sha256(random()::text || clock_timestamp()::text) which works on all PG14+.
			db.Pool.Exec(context.Background(), `CREATE OR REPLACE FUNCTION uuid_generate_v7() RETURNS uuid AS $$
DECLARE unix_ts_ms bytea; uuid_bytes bytea; rand_bytes bytea;
BEGIN
  unix_ts_ms = substring(int8send(floor(extract(epoch from clock_timestamp()) * 1000)::bigint) from 3);
  BEGIN
    rand_bytes = gen_random_bytes(10);
  EXCEPTION WHEN undefined_function THEN
    rand_bytes = substring(sha256((random()::text || clock_timestamp()::text)::bytea) FROM 1 FOR 10);
  END;
  uuid_bytes = unix_ts_ms || rand_bytes;
  uuid_bytes = set_byte(uuid_bytes, 6, (b'0111' || get_byte(uuid_bytes, 6)::bit(4))::bit(8)::int);
  uuid_bytes = set_byte(uuid_bytes, 8, (b'10' || get_byte(uuid_bytes, 8)::bit(6))::bit(8)::int);
  RETURN encode(uuid_bytes, 'hex')::uuid;
END $$ LANGUAGE plpgsql VOLATILE`)

			// Auto-migrate on startup. Try disk locations first (allows
			// operators to ship hotfix SQL files without a full binary
			// redeploy). Fall back to the FS embedded in the binary at
			// build time — this is what makes fresh installs work with
			// no external files.
			migDir := "migrations"
			if _, err := os.Stat(migDir); os.IsNotExist(err) {
				exe, _ := os.Executable()
				migDir = filepath.Join(filepath.Dir(exe), "migrations")
			}
			if _, err := os.Stat(migDir); os.IsNotExist(err) {
				home, _ := os.UserHomeDir()
				migDir = filepath.Join(home, ".qorven", "migrations")
			}
			if err := db.MigrateUpFS(embeddedMigrations, migDir); err != nil {
				slog.Warn("migration failed (non-fatal)", "error", err, "dir", migDir)
			}

			// Ensure default tenant exists
			db.Pool.Exec(context.Background(),
				`INSERT INTO tenants (id, name, slug) VALUES ($1, 'Default', 'default') ON CONFLICT (id) DO NOTHING`,
				defaultTenant)

			// Attach DB to daemon registry — restores pending plans/tasks on boot.
			gw.daemonReg.SetPool(db.Pool, defaultTenant)

			gw.agents = agent.NewStore(db.Pool)
			gw.sessions = session.NewStore(db.Pool)
			gw.providerStore = providers.NewStore(db.Pool, cfg.Auth.EncryptionKey)
			gw.voiceStore = voice.NewStore(db.Pool, cfg.Auth.EncryptionKey)
			gw.mediaStore = mediagen.NewStore(db.Pool, cfg.Auth.EncryptionKey)
			gw.skillStore = skills.NewStore(db.Pool)
			// Install the built-in skill library on boot. Idempotent —
			// re-installing updates the canonical copies on disk
			// without overwriting user edits. Errors are logged but
			// non-fatal; a missing prompts folder shouldn't block
			// the gateway from starting.
			{
				home, _ := os.UserHomeDir()
				builtinRoot := filepath.Join(home, ".qorven", "skills", "builtin")
				inst, skip, errs := gw.skillStore.InstallBuiltIns(context.Background(), defaultTenant, builtinRoot)
				slog.Info("skills.builtin_installed",
					"installed", inst, "skipped_unchanged", skip, "errors", len(errs))
				for _, e := range errs {
					slog.Warn("skills.builtin_error", "error", e)
				}
			}
			gw.notifStore = notifications.NewStore(db.Pool)
			gw.auditStore = audit.NewStore(db.Pool)
			gw.discussionStore = discussion.NewStore(db.Pool)
			// Wire no-op embedder for now — topic drift detection without vector embeddings.
			// LabelGenerator uses a Chat call to generate concise labels.
			gw.clusterer = discussion.NewClusterer(
				db.Pool,
				discussion.StubEmbedder{Vec: nil}, // nil vec → always extend current discussion
				&gatewayLabeller{gw: gw},
			)
			gw.billingStore = billing.NewStore(db.Pool)
			gw.bundleStore = agent.NewBundleStore(db.Pool)
			gw.customTools = tools.NewCustomToolStore(db.Pool, cfg.Auth.EncryptionKey)
			gw.memStore = memory.NewStore(db.Pool)
			gw.dreamer = memory.NewDreamer(gw.memStore, defaultTenant, 6*time.Hour)
			gw.kgStore = knowledgegraph.NewStore(db.Pool)
			gw.taskStore = tasks.NewStore(db.Pool)
			gw.wfStore = workflow.NewStore(db.Pool)
			gw.msgStore = agent.NewMessageStore(db.Pool)
			gw.hbStore = heartbeat.NewStore(db.Pool)
			gw.mailStore = mail.NewStore(db.Pool)
			gw.mailRouter = mail.NewRouter(gw.mailStore)
			gw.driveStore = drive.NewStore(db.Pool)
			gw.rtHub.SetPresence(presence.NewStore(db.Pool))
			gw.sandboxStore = sandbox.NewStore(db.Pool)
			gw.appRunner = sandbox.NewAppRunner(db.Pool, cfg.Server.BaseURL)
			gw.calendarStore = calendar.NewStore(db.Pool)
			gw.connKB = connectors.NewKnowledgeStore(db.Pool)
			gw.vault = vault.New(db.Pool, cfg.Auth.EncryptionKey)
			baseURL := cfg.Server.BaseURL
			if baseURL == "" {
				baseURL = "http://localhost"
			}
			gw.oauthMgr = oauth.NewManager(gw.vault, baseURL)
			oauth.RegisterDefaults(gw.oauthMgr, nil)
			// Pull any per-tenant OAuth-app credentials the user
			// previously registered via Settings → Connectors.
			// Without this, every reboot would reset their Slack /
			// GitHub / Google app settings back to the env defaults
			// (which self-hosted users typically don't have).
			gw.hydrateOAuthAppsFromVault(context.Background())
			gw.connExec = connectors.NewExecutor(gw.connKB, gw.vault, defaultTenant)
			connectors.SeedPlatforms(context.Background(), gw.connKB)
			gw.mcpManager = mcp.NewManager(db.Pool, gw.mcpClient)
			gw.loadProvidersFromDB()
		}
	}

	// Intelligence enrichment boot — LLM Stats and Artificial Analysis.
	// Keys come from (in priority order):
	//   1. DB (provider_configs — set via GUI/TUI, encrypted at rest)
	//   2. config.toml
	// This allows end-users to set keys from the UI without touching files.
	if gw.db != nil {
		if key := gw.loadIntegrationKey(context.Background(), "llmstats"); key != "" {
			cfg.LLMStats.APIKey = key
		}
		if key := gw.loadIntegrationKey(context.Background(), "artificialanalysis"); key != "" {
			cfg.ArtificialAnalysis.APIKey = key
		}
	}

	// LLM Stats enrichment loop.
	if cfg.LLMStats.APIKey != "" && gw.db != nil {
		hours := cfg.LLMStats.RefreshHours
		if hours <= 0 {
			hours = 24
		}
		gw.startLLMStatsLoop(cfg.LLMStats.APIKey, hours)
	}

	// Artificial Analysis enrichment loop.
	if cfg.ArtificialAnalysis.APIKey != "" && gw.db != nil {
		hours := cfg.ArtificialAnalysis.RefreshHours
		if hours <= 0 {
			hours = 24
		}
		gw.aaClient = artificialanalysis.New(cfg.ArtificialAnalysis.APIKey)
		maxAge := time.Duration(hours) * time.Hour
		go func() {
			artificialanalysis.RefreshAndMerge(context.Background(), gw.aaClient, gw.db.Pool, maxAge)
			t := time.NewTicker(maxAge)
			defer t.Stop()
			for range t.C {
				artificialanalysis.RefreshAndMerge(context.Background(), gw.aaClient, gw.db.Pool, maxAge)
			}
		}()
	}

	// Config.toml providers (fallback)
	for _, p := range cfg.Providers {
		if p.Type == "bedrock" {
			bp, err := providers.NewBedrockProvider(p.Name, p.Model, p.Region)
			if err != nil {
				slog.Warn("bedrock provider failed", "name", p.Name, "error", err)
				continue
			}
			gw.providerReg.RegisterProvider(p.Name, bp)
			slog.Info("bedrock provider registered", "name", p.Name, "model", p.Model, "region", p.Region, "default", p.IsDefault)
			continue
		}
		gw.providerReg.Register(providers.ProviderConfig{
			ID: "config-" + p.Name, Name: p.Name, ProviderType: p.Type,
			APIBase: p.APIBase, APIKey: p.APIKey, Enabled: true,
		})
	}

	// Register ALL tools (chanMgr wired later via SetChannelSender)
	gw.registerTools()

	// Create agent loop (the core engine)
	if gw.agents != nil {
		gw.agentLoop = agent.NewLoop(
			gw.agents, gw.sessions, gw.providerReg,
			gw.toolReg, gw.skillLoader, gw.memStore, defaultTenant,
		)
		if gw.skillStore != nil {
			gw.agentLoop.SetSkillStore(gw.skillStore)
		}
		ragPipeline := rag.NewPipeline(gw.db.Pool, gw.memStore, nil, defaultTenant, gw.getEmbeddingURL())
		gw.agentLoop.Hooks = agent.NewHookChain(
			&agent.LoggingHook{},
			agent.NewMetricsHook(),
			agent.NewBudgetHook(gw.agents),
			agent.NewKnowledgeHook(ragPipeline, gw.agents),
			agent.NewPlanModeHook(gw.agentLoop),
			agent.NewAutoCompactHook(gw.agentLoop),
			agent.NewPermissionHook(permissions.ModeDefault),
		)
		gw.agentLoop.OnMessage = func(sessionID, agentID, role, content, channel string) {
			gw.rtHub.BroadcastNewMessage(sessionID, agentID, role, content, channel)
			// Live activity: broadcast agent activity
			gw.rtHub.BroadcastSoulActivity(agentID, "", role, content[:min(len(content), 100)])
		}

		// PII redaction. Read the tenant toggle from system_configs on
		// boot; Settings → Privacy toggles rewrite this row, and a
		// subsequent config reload picks up the new state. Off by
		// default — we never silently redact user content.
		if gw.db != nil && gw.db.Pool != nil {
			if kinds, on := loadPIIKinds(context.Background(), gw.db.Pool, defaultTenant); on && kinds != 0 {
				gw.agentLoop.SetPIIRedactor(agent.NewPIIRedactor(kinds))
				slog.Info("pii.redaction.enabled", "tenant", defaultTenant, "kinds", kinds)
			}
			// Prompt-injection defense — policy is off/warn/block/strict.
			// Hot-reloadable on prefs save (see handleSavePreferences).
			policy := loadPromptGuardPolicy(context.Background(), gw.db.Pool, defaultTenant)
			gw.agentLoop.SetPromptGuardPolicy(policy)
			if policy != agent.PromptGuardOff {
				slog.Info("promptguard.enabled", "tenant", defaultTenant, "policy", policy)
			}
		}
		// OpenSpace: wire skill crystallizer for self-evolving skills
		defaultProvider := gw.providerReg.Default()
		if defaultProvider != nil {
			gw.agentLoop.Crystallizer = skills.NewCrystallizer(gw.db.Pool, defaultProvider, "default")
		}
		// SmartRouter — only routes if agent model is "auto" or empty
		gw.agentLoop.SmartRouter = providers.NewSmartRouter(gw.db.Pool)
		gw.agentLoop.SmartRouter.SetRegistry(gw.providerReg)
		go gw.seedModelDefaults(context.Background())
		gw.agentLoop.Events = agent.NewEventBus()
		gw.agentLoop.ModelSwitchQ = providers.NewModelSwitchQueue()
		gw.agentLoop.PermGate = gw.permissionGate

		// Audit: log every tool execution to audit_log
		if gw.auditStore != nil {
			gw.agentLoop.SetAuditFn(func(agentKey, toolName, sessionID string, isError bool) {
				action := "tool_exec"
				if isError {
					action = "tool_error"
				}
				gw.auditStore.Log(context.Background(), defaultTenant, "agent", agentKey, agentKey, action, "tool", toolName, map[string]any{
					"session_id": sessionID, "is_error": isError,
				}, "")
			})
		}

		// Wire project registry after tools are registered (deferred)
		defer func() {
			if gw.projectReg != nil {
				gw.agentLoop.SetProjectRegistry(gw.projectReg)
			}
		}()

		// Supervisor Protocol — Prime watches all Qors
		// Supervisor DB store
		supervisorStore := supervisorpkg.NewStore(gw.db.Pool)
		gw.supervisorBus = supervisorpkg.NewBus(func(msg supervisorpkg.Message) {
			// Escalation callback — notify human via realtime hub + notifications
			slog.Info("supervisor.escalation", "from", msg.From, "risk", msg.Risk, "content", msg.Content)
			if gw.rtHub != nil {
				gw.rtHub.BroadcastNewMessage("", "", "system", "⚠️ Escalation: "+msg.Content)
			}
			// Create persistent notification
			gw.writeNotification(msg.From, "", "Prime", "escalation", "Escalation: "+msg.Content[:min(len(msg.Content), 80)], msg.Content, "supervisor", msg.ID)
		})
		gw.supervisorBus.SetOnMessage(func(msg supervisorpkg.Message) {
			supervisorStore.SaveMessage(context.Background(), msg)
		})
		// Gap 4 fix: restore pending supervisor reviews from DB on startup.
		// Any escalations pending before the restart are re-delivered to the escalation handler.
		if err := gw.supervisorBus.RestoreFromDB(context.Background(), supervisorStore); err != nil {
			slog.Warn("supervisor.restore_failed", "error", err)
		}
		fixDeps := supervisorpkg.FixDependencies{
			RestartCron: func(ctx context.Context, jobID string) error {
				if gw.cronRunner != nil {
					return gw.cronRunner.RestartJob(ctx, jobID)
				}
				return fmt.Errorf("cron runner not available")
			},
			SwitchModel: func(ctx context.Context, agentID, newModel string) error {
				if gw.agentLoop.ModelSwitchQ != nil {
					gw.agentLoop.ModelSwitchQ.SwitchModel(agentID, newModel)
				}
				return nil
			},
			ResetSession: func(ctx context.Context, sessionID string) error {
				if gw.sessions != nil {
					return gw.sessions.Delete(ctx, sessionID)
				}
				return nil
			},
			ClearCache: func(ctx context.Context, key string) error {
				// Clear memory cache for the agent
				if gw.memStore != nil {
					_, err := gw.memStore.Decay(ctx, key)
					return err
				}
				return nil
			},
			RestartChannel: func(ctx context.Context, channelID string) error {
				if gw.chanMgr != nil {
					gw.chanMgr.Stop(ctx, channelID)
					return gw.chanMgr.Start(ctx, channelID)
				}
				return nil
			},
		}
		fixCatalog := supervisorpkg.NewFixCatalog(fixDeps)
		// Find Prime (chief) agent ID — seeding is handled by Start() → ensureChief()
		primeID := ""
		if gw.agents != nil {
			if chief, err := gw.agents.GetByKey(context.Background(), "chief"); err == nil && chief != nil {
				primeID = chief.ID
			}
		}
		if primeID != "" {
			gw.supervisor = supervisorpkg.NewSupervisor(gw.supervisorBus, fixCatalog, supervisorpkg.DefaultConfig(), primeID)
			gw.supervisor.SetListAgents(func(ctx context.Context) ([]supervisorpkg.AgentInfo, error) {
				agents, err := gw.agents.List(ctx, defaultTenant)
				if err != nil {
					return nil, err
				}
				infos := []supervisorpkg.AgentInfo{}
				for _, a := range agents {
					infos = append(infos, supervisorpkg.AgentInfo{ID: a.ID, Name: a.DisplayName, Key: a.AgentKey, Model: a.Model, FallbackModel: a.FallbackModel})
				}
				return infos, nil
			})
			// Wire evaluator — uses cheap model to judge outputs
			defaultProvider := gw.providerReg.Default()
			if defaultProvider != nil {
				evaluator := supervisorpkg.NewEvaluator(defaultProvider, "")
				gw.supervisor.SetEvaluator(func(ctx context.Context, agentID, output string) (*supervisorpkg.EvalResult, error) {
					return evaluator.Evaluate(ctx, agentID, "", "", output)
				})
			}
			gw.supervisor.Start(context.Background())
			slog.Info("supervisor.started", "prime", primeID, "evaluator", defaultProvider != nil)

			// Wire supervisor suspension check into daemon registry so suspended agents
			// cannot receive new tasks until a human clears them.
			if gw.daemonReg != nil {
				gw.daemonReg.SetSuspensionChecker(gw.supervisor)
			}

			// Wire supervisor bus into agent loop
			gw.agentLoop.SupervisorBus = gw.supervisorBus
			gw.agentLoop.PrimeID = primeID

			// Wire Prime delegation — allows Prime to execute specialist agents
			pd := agent.NewPrimeDelegation(primeID, gw.agents)
			pd.SetRunAgent(func(ctx context.Context, agentID, message string) (string, error) {
				return gw.agentLoop.Chat(ctx, agentID, message)
			})
			pd.SetOnComplete(func(task *agent.DelegatedTask) {
				slog.Info("delegation.complete", "task", task.ID, "specialist", task.SpecialistKey,
					"status", task.Status)
			})
			gw.agentLoop.SetPrimeDelegation(pd)
			slog.Info("prime.delegation.enabled", "prime", primeID)
			// Wire memory hierarchy + working memory
			if gw.memStore != nil {
				hierMem := memory.NewHierarchyStore(gw.memStore, defaultTenant)
				gw.agentLoop.HierarchyMem = hierMem
				gw.agentLoop.WorkingMem = memory.NewWorkingMemory()
				gw.agentLoop.KnowledgeGraph = memory.NewKnowledgeGraph(gw.db.Pool)

				// Gap B fix: Prime Live Digest — Prime knows what every agent is doing
				if primeID != "" && gw.sessions != nil && gw.brain != nil {
					pd := agent.NewPrimeDigest(gw.agents, gw.sessions, hierMem, primeID, 5*time.Minute)
					gw.brain.PrimeDigest = pd
				}
				// Generate system knowledge dynamically from codebase
				backendDir := "."
				frontendDir := "../web"
				knowledge := systempkg.GenerateSystemKnowledge(context.Background(), backendDir, frontendDir, "config.toml")
				// Trim to 2000 chars to avoid bloating the system prompt
				if len(knowledge) > 2000 {
					knowledge = knowledge[:2000] + "\n... (truncated for performance)"
				}
				gw.agentLoop.SetSystemKnowledge(knowledge)
				slog.Info("system.knowledge.generated", "size", len(knowledge))

				// Code change pipeline — safe self-modification
				gw.codePipeline = systempkg.NewPipeline(".")
				gw.codePipeline.SetReviewCallback(func(change *systempkg.CodeChange) {
					// Send to supervisor for review
					if gw.supervisorBus != nil {
						gw.supervisorBus.Send(context.Background(), supervisorpkg.Message{
							From:    change.ProposedBy,
							To:      primeID,
							Intent:  supervisorpkg.IntentReviewRequest,
							Content: fmt.Sprintf("Code change proposed: %s (%d files, risk=%s)", change.Description, len(change.Files), change.Risk),
							Risk:    supervisorpkg.RiskLevel(change.Risk),
							Context: map[string]any{"change_id": change.ID, "files": len(change.Files), "compile_ok": change.CompileOK, "tests_passed": change.TestsPassed},
						})
					}
					// Notification
					gw.writeNotification("", "", "Pipeline", "pipeline", "Code change validated: "+change.Description, fmt.Sprintf("%d files, %d tests passed", len(change.Files), change.TestsPassed), "pipeline", change.ID)
				})
				slog.Info("pipeline.initialized")
			}
			// Schedule daily memory decay
			if gw.memStore != nil {
				go memory.ScheduleDecay(context.Background(), gw.memStore, defaultTenant, 24*time.Hour)
			}
		}

		// Scenario Lab
		if gw.db != nil {
			scenarioStore := scenario.NewStore(gw.db.Pool)
			gw.scenarioHandlers = scenario.NewHandlers(scenarioStore, gw.providerReg.Default())
		}
		gw.agentLoop.ModelSwitchQ = providers.NewModelSwitchQueue()
		gw.agentLoop.WebAugmenter = webintel.NewAugmenter(webintel.New(cfg.SearxngURL))
		gw.agentLoop.SetConnectorKB(gw.connKB, gw.mcpManager)
		if gw.billingStore != nil {
			gw.agentLoop.SetBillingStore(gw.billingStore)
		}
		if gw.bundleStore != nil {
			gw.agentLoop.SetBundleStore(gw.bundleStore)
		}

		// Wire new core systems
		// Plugin system
		gw.pluginMgr = plugin.NewManager()
		pluginMgr := gw.pluginMgr
		pluginMgr.LoadDir(filepath.Join(os.Getenv("HOME"), ".qorven", "plugins"))
		gw.agentLoop.SetPluginManager(pluginMgr)
		slog.Info("plugins loaded", "count", len(pluginMgr.List()))

		// App Platform — load installed apps, wire their tools/hooks.
		if gw.db != nil {
			appStore := apps.NewStore(gw.db.Pool)
			// Hoist KeyPoolStore allocation once; captured by the credLookup closure below.
			credKS := providers.NewKeyPoolStore(gw.db.Pool, gw.cfg.Auth.EncryptionKey)
			gw.appMgr = apps.NewAppManager(appStore, gw.toolReg, gw.pluginMgr, gw.db.Pool, defaultTenant,
				func(tenantID, slug string) string {
					// db is guaranteed non-nil; closure constructed inside gw.db != nil guard
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					keys, _ := credKS.ListKeys(ctx, tenantID, slug)
					for _, k := range keys {
						if k.Status == "verified" {
							if b, err := providers.DecryptKeyBytes(k.EncryptedKey(), gw.cfg.Auth.EncryptionKey); err == nil {
								return b
							}
						}
					}
					return ""
				},
			)
			if err := gw.appMgr.LoadAll(context.Background(), "", ""); err != nil {
				slog.Warn("apps.load_all_failed", "err", err)
			}

			// scaffold_app / install_app — let the agent build and install apps.
			home, _ := os.UserHomeDir()
			appsDir := filepath.Join(home, ".qorven", "apps")
			gw.toolReg.Register(tools.NewScaffoldAppTool(appsDir))
			appMgr := gw.appMgr
			gw.toolReg.Register(tools.NewInstallAppTool(
				func(ctx context.Context, manifestDir string) (string, string, string, error) {
					a, err := appMgr.Install(ctx, manifestDir)
					if err != nil {
						return "", "", "", err
					}
					return a.ID, a.Slug, a.DisplayName, nil
				},
				func(ctx context.Context, slug string) error {
					return appMgr.Reload(ctx, slug)
				},
			))
			gw.toolReg.Register(tools.NewUninstallAppTool(
				func(ctx context.Context, slug string) (string, error) {
					a, err := appMgr.Store().GetBySlug(ctx, defaultTenant, slug)
					if err != nil {
						return "", err
					}
					return a.ID, nil
				},
				func(ctx context.Context, id string, dropTables bool) error {
					return appMgr.Uninstall(ctx, id, dropTables)
				},
			))
			gw.toolReg.Register(tools.NewBuildConnectorTool(
				func(ctx context.Context, manifestDir string) (string, string, string, error) {
					a, err := appMgr.Install(ctx, manifestDir)
					if err != nil {
						return "", "", "", err
					}
					return a.ID, a.Slug, a.DisplayName, nil
				},
				func(ctx context.Context, slug, toolName string, args map[string]any) (*tools.Result, error) {
					return appMgr.RunTool(ctx, slug, toolName, args)
				},
			))
		}

		// Self-improving skill learner
		if gw.skillStore != nil {
			learner := skills.NewLearner(gw.skillStore, gw.skillLoader, defaultTenant, "")
			gw.agentLoop.SetSkillLearner(learner)
			slog.Info("skill learner enabled")
		}

		// Per-agent channel bindings
		if gw.db != nil {
			gw.bindingStore = channels.NewBindingStore(gw.db.Pool)
			slog.Info("per-agent channel bindings enabled")
		}

		// Prompt cache (5 minute TTL)
		gw.agentLoop.SetPromptCache(agent.NewPromptCache(5 * time.Minute))
		// Check global toggle — if web search disabled globally, nil out augmenter
		if gw.db != nil {
			var enabled string
			gw.db.Pool.QueryRow(context.Background(),
				`SELECT COALESCE(value,'true') FROM system_configs WHERE tenant_id = $1 AND key = 'services.web_search.enabled'`,
				defaultTenant).Scan(&enabled)
			if enabled == "false" {
				gw.agentLoop.WebAugmenter = nil
				slog.Info("web_search globally disabled")
			}
		}
		slog.Info("agent loop initialized")

		// AG-UI handler — wired to the agent loop so POST /v1/agui/stream
		// can stream spec-compliant events to AG-UI framework clients.
		gw.aguiHandler = agui.New(gw.agentLoop)
		if gw.sessions != nil {
			gw.aguiHandler.SetSessionStore(gw.sessions)
		}

		// Initialize the Universal Brain Engine. MemoryDir defaults to
		// ~/.qorven/brain-memory so fresh installs don't collide and
		// multi-tenant deployments can point it at a volume. Users can
		// override by setting QORVEN_MEMORY_DIR before start.
		memoryDir := os.Getenv("QORVEN_MEMORY_DIR")
		if memoryDir == "" {
			home, _ := os.UserHomeDir()
			memoryDir = filepath.Join(home, ".qorven", "brain-memory")
		}
		gw.brain = engine.New(engine.Options{
			ConfigPath:   cfg.ConfigPath,
			MemoryDir:    memoryDir,
			TenantID:     defaultTenant,
			AgentStore:   gw.agents,
			SessionStore: gw.sessions,
			ProviderReg:  gw.providerReg,
			ToolReg:      gw.toolReg,
			SkillLoader:  gw.skillLoader,
			MemStore:     gw.memStore,
		})
		gw.brain.Loop = gw.agentLoop // share the same loop instance
		gw.brain.Start()             // start cron + heartbeat
		slog.Info("brain engine initialized and started")

		// Ensure persisted cron jobs have next_run_at set so gw.cronRunner will pick them up.
		// gw.brain.Cron is NOT used for user jobs — only gw.cronRunner (DB-backed) executes them.
		if gw.db != nil {
			go func() {
				_, err := gw.db.Pool.Exec(context.Background(),
					`UPDATE cron_jobs SET next_run_at = NOW() + interval '1 minute'
					 WHERE enabled = true AND next_run_at IS NULL`)
				if err != nil {
					slog.Warn("cron.boot_next_run_set_failed", "err", err)
				} else {
					slog.Info("cron.boot_next_run_initialized")
				}
			}()
		}

		// Data source scheduler — reads installed connector apps, registers cron jobs,
		// and snapshots tool output for briefing injection.
		var snapStore *datasource.SnapshotStore
		if gw.db != nil && gw.appMgr != nil {
			snapStore = datasource.NewSnapshotStore(gw.db.Pool)
			gw.snapStore = snapStore
			gw.tileStore = dashboard.NewTileStore(gw.db.Pool)
			gw.dsScheduler = datasource.NewScheduler(
				gw.db.Pool,
				snapStore,
				gw.appMgr.RunTool,
				defaultTenant,
			)
			go func() {
				if err := gw.dsScheduler.SyncAll(context.Background()); err != nil {
					slog.Error("datasource.scheduler.sync_failed", "err", err)
				}
			}()
		}

		// Daily briefing scheduler — fires at the wall-clock time each agent configured.
		briefingBuilder := briefing.NewBuilder(gw.db.Pool, gw.agentLoop, snapStore)
		briefingCron := autonomy.NewCronScheduler(func(ctx context.Context, job *autonomy.CronJob) (*autonomy.CronRunResult, error) {
			err := briefingBuilder.Deliver(ctx, job.AgentID, job.TenantID)
			return &autonomy.CronRunResult{AgentID: job.AgentID, JobID: job.ID, Success: err == nil}, err
		})
		briefingCron.Start()
		gw.briefingSched = briefing.NewScheduler(briefingCron, briefingBuilder, gw.db.Pool)
		if err := gw.briefingSched.SyncAll(context.Background()); err != nil {
			slog.Warn("briefing.sync_all_failed", "err", err)
		}

		// Boot persistent agent runtimes (migration 071).
		if gw.agents != nil && gw.taskStore != nil {
			agentList, err := gw.agents.List(context.Background(), defaultTenant)
			if err == nil {
				gw.runtimeMgr = agent.NewRuntimeManager(context.Background(), gw.dispatchRuntimeSignal)
				gw.taskCoordinator = NewTaskCoordinator(gw.taskStore, gw.runtimeMgr, gw.rtHub)
				if gw.db != nil {
					gw.taskCoordinator.SetPresence(presence.NewStore(gw.db.Pool))
				}
				var bootEntries []agent.AgentBootEntry
				for _, a := range agentList {
					bootEntries = append(bootEntries, agent.AgentBootEntry{
						AgentID:     a.ID,
						TenantID:    a.TenantID,
						RuntimeMode: a.RuntimeMode,
					})
				}
				gw.runtimeMgr.StartAll(bootEntries)
				slog.Info("persistent runtimes booted", "agents", len(bootEntries))
			}
		}

		// SoulDesk — multi-Soul orchestration
		desk := souldesk.New(gw.agents, gw.sessions, gw.providerReg, gw.toolReg, gw.skillLoader, gw.memStore, defaultTenant)
		gw.soulDesk = desk
		gw.toolReg.Register(souldesk.NewDelegateTool(desk))
		gw.toolReg.Register(souldesk.NewListSoulsTool(desk))
		gw.toolReg.Register(souldesk.NewCreateSoulTool(desk))
		gw.toolReg.Register(souldesk.NewCheckUpdatesTool(desk))
		gw.toolReg.Register(souldesk.NewSoulMessageTool(desk))
		gw.toolReg.Register(souldesk.NewHandoffTool(desk))
		gw.toolReg.Register(souldesk.NewDispatchTeamTasksTool(desk))

		// Task integration — delegation creates Kanban tasks
		if gw.taskStore != nil {
			ti := souldesk.NewTaskIntegration(gw.taskStore, defaultTenant)
			desk.SetTaskIntegration(ti)
			if gw.skillStore != nil {
				desk.SetSkillStore(gw.skillStore)
			}
			gw.toolReg.Register(souldesk.NewPickupTaskTool(ti))
		}

		// A2A Protocol server — hostname + port only. Prefer the
		// concrete API listener address (APIListen), then fall back
		// to the legacy Listen field, then to a safe default. The
		// previous `cfg.Server.Listen[strings.LastIndex(...):]`
		// expression panicked with a [-1:] slice when Listen was empty
		// (the normal case once deployments move to APIListen).
		apiAddr := firstNonEmpty(cfg.Server.APIListen, cfg.Server.Listen, "127.0.0.1:4200")
		apiPort := apiAddr
		if i := strings.LastIndex(apiAddr, ":"); i >= 0 {
			apiPort = apiAddr[i:] // e.g. ":4200"
		}
		baseURL := "http://localhost" + apiPort
		gw.a2aServer = a2a.NewServer(gw.agents, baseURL, func(ctx context.Context, agentID, message string) (string, error) {
			return gw.agentLoop.Chat(ctx, agentID, message)
		})
		slog.Info("a2a protocol server initialized")

		// Wire real-time hub for live Qor activity streaming (P3.1)
		desk.SetRTHub(gw.rtHub)
		if gw.agentLoop != nil && gw.agentLoop.SmartRouter != nil {
			desk.SetSmartRouter(gw.agentLoop.SmartRouter)
		}

		// Prime Qor follow-up: when a Soul completes, deliver result to chat
		desk.OnDelegationComplete = func(ctx context.Context, primeID, sessionID, soulKey, task, result string) {
			// Short results: deliver directly — no Prime Qor follow-up needed
			if len(result) < 500 {
				msg := fmt.Sprintf("✅ @%s completed: %s\n\n%s", soulKey, task[:min(len(task), 80)], result)
				if gw.sessions != nil && sessionID != "" {
					gw.sessions.AppendMessage(ctx, sessionID, session.Message{
						Role: "assistant", Content: msg, Timestamp: time.Now().UnixMilli(),
					}, 0, 0)
				}
				gw.rtHub.BroadcastNewMessage(sessionID, primeID, "assistant", msg)
				return
			}

			// Long results: Prime Qor summarizes briefly (hide internal prompt)
			if gw.agentLoop == nil {
				return
			}
			prompt := fmt.Sprintf("[SYSTEM: @%s finished. Summarize this in 2-3 sentences for the user. Do NOT repeat the full report.]\n\nTask: %s\nResult from @%s:\n%s",
				soulKey, task[:min(len(task), 100)], soulKey, result[:min(len(result), 1500)])

			// Save only as system message (hidden from user), not as user message
			if gw.sessions != nil && sessionID != "" {
				gw.sessions.AppendMessage(ctx, sessionID, session.Message{
					Role: "system", Content: fmt.Sprintf("[Report from @%s received]", soulKey), Timestamp: time.Now().UnixMilli(),
				}, 0, 0)
			}

			gw.agentLoop.Run(ctx, agent.RunRequest{
				AgentID:     primeID,
				SessionID:   sessionID,
				UserMessage: prompt,
				Channel:     "internal", // marks this as internal — loop can skip saving user msg
			}, func(event agent.StreamEvent) {})
		}

		// @mention router
		gw.mentionRouter = souldesk.NewMentionRouter(gw.agents, defaultTenant)

		// Workflow executor
		if gw.wfStore != nil {
			gw.wfExecutor = workflow.NewExecutor(gw.wfStore, gw.providerReg, gw.toolReg, defaultTenant)
		}

		// Research engine — bridge the provider registry into the llm.Provider
		// surface research expects. Without this, synthesize() panics on a
		// nil llm and (after the nil-guard) falls back to raw source dumps.
		gw.researchEngine = research.NewEngine(cfg.SearxngURL, research.NewLLMAdapter(gw.providerReg))

		slog.Info("souldesk initialized", "tools", "delegate_to_soul, list_souls, create_soul, check_tasks")
	}

	// Inbound processor — classifies and routes external channel messages
	if gw.db != nil {
		gw.inbound = inbound.NewProcessor(gw.db.Pool, gw.sessions, gw.agentLoop)
		if gw.pluginMgr != nil {
			gw.inbound.SetPluginManager(gw.pluginMgr)
		}
		gw.inbound.SetReplyFunc(func(ctx context.Context, agentID, channelType, chatID, content string, metadata map[string]string) {
			gw.sendToChannelByTypeWithMeta(ctx, agentID, channelType, chatID, content, "", metadata["message_id"], nil, metadata)
		})
	}

	// Channel manager — routes inbound messages to the right Soul's agent loop
	var policyChecker *channels.PolicyChecker
	if gw.db != nil {
		policyChecker = channels.NewPolicyChecker(gw.db.Pool)
	}
	dedup := NewInboundDedup(20*time.Minute, 5000)
	gw.chanMgr = channels.NewManager(func(ctx context.Context, msg channels.InboundMessage) {
		if gw.agentLoop == nil || msg.AgentID == "" {
			return
		}

		// Dedup: skip duplicate messages from webhook retries
		if msgID := msg.Metadata["message_id"]; msgID != "" {
			key := fmt.Sprintf("%s|%s|%s|%s", msg.ChannelType, msg.SenderID, msg.Metadata["chat_id"], msgID)
			if dedup.IsDuplicate(key) {
				slog.Debug("dedup.skip", "key", key)
				return
			}
		}

		// Handle commands (/reset, /stop, subagent announce) before normal processing
		if gw.handleChannelCommand(ctx, msg) {
			return
		}

		// DM Policy check — pairing/allowlist/open/disabled
		var dmPolicy string
		allowlist := []string{}
		gw.db.Pool.QueryRow(ctx,
			`SELECT COALESCE(dm_policy,'pairing'), COALESCE(allowlist,'{}') FROM channel_instances WHERE agent_id = $1 AND channel_type = $2 AND enabled = true LIMIT 1`,
			msg.AgentID, msg.ChannelType).Scan(&dmPolicy, &allowlist)

		allowed, replyMsg := policyChecker.CheckDM(ctx, defaultTenant, msg.ChannelType, msg.SenderID, msg.SenderName, dmPolicy, allowlist)
		if !allowed {
			if replyMsg != "" {
				chatID := msg.Metadata["chat_id"]
				if chatID == "" {
					chatID = msg.SenderID
				}
				gw.sendToChannel(ctx, msg.AgentID, chatID, replyMsg, "", "")
			}
			return
		}

		// Record approved sender so proactive messages (cron, reminders) can reach them later
		if msg.ChannelType == "telegram" && gw.db != nil {
			chatID := msg.Metadata["chat_id"]
			if chatID == "" {
				chatID = msg.SenderID
			}
			gw.db.Pool.Exec(ctx,
				`INSERT INTO paired_devices (tenant_id, channel, sender_id, chat_id, sender_name)
				 VALUES ($1, 'telegram', $2, $3, $4) ON CONFLICT DO NOTHING`,
				defaultTenant, msg.SenderID, chatID, msg.SenderName)
		}

		// Resolve user ID — group-scoped for groups, sender ID for DMs
		userID := msg.SenderID
		isGroup := msg.Metadata["peer_kind"] == "group"
		if isGroup {
			userID = buildGroupUserID(msg)
		}

		// Persist session metadata (display_name, username, chat_title)
		if meta := extractSessionMetadata(msg); meta != nil && gw.sessions != nil {
			// Will be stored when session is created/found
			_ = meta
		}

		// One-Qor-one-chat: for 1:1 chat-family surfaces (telegram,
		// whatsapp, slack_dm, discord_dm, webchat) we collapse every
		// inbound message into the Qor's single canonical session —
		// stored under channel="web" by convention. The per-message
		// Channel tag (set below) preserves which transport it came
		// from so the UI can render badges and per-channel surfaces
		// can filter to their own slice.
		//
		// Email, group chats, voice, and other non-chat-family
		// channels keep their own session rows (isolated per
		// channel) — each is genuinely a different thread/thread
		// type.
		var sessionID string
		if gw.sessions != nil {
			lookupChannel := msg.ChannelType
			if !isGroup && isChatFamilyChannel(msg.ChannelType) {
				lookupChannel = "web"
			}
			sess, err := gw.sessions.FindByAgentAndChannel(ctx, msg.AgentID, lookupChannel)
			if err == nil && sess != nil {
				sessionID = sess.ID
			} else {
				newSess, err := gw.sessions.Create(ctx, defaultTenant, msg.AgentID, userID, lookupChannel)
				if err == nil {
					sessionID = newSess.ID
				}
			}
		}

		// Store the inbound message with channel tag
		if gw.sessions != nil && sessionID != "" {
			gw.sessions.AppendMessage(ctx, sessionID, session.Message{
				Role: "user", Content: msg.Content, Timestamp: time.Now().UnixMilli(),
				Channel: msg.ChannelType, SenderName: msg.SenderName,
			}, 0, 0)
			// Broadcast inbound message to Web UI via WebSocket
			if gw.rtHub != nil {
				gw.rtHub.BroadcastNewMessage(sessionID, msg.AgentID, "user", msg.Content, msg.ChannelType, msg.SenderName)
			}
		}

		chatID := msg.Metadata["chat_id"]
		if chatID == "" {
			chatID = msg.SenderID
		}

		// Build run request with all context.
		// TenantID and a valid UUID UserID are required so the permission gate
		// can find auto-approved policies. Channel sessions (Telegram etc.) carry
		// a non-UUID SenderID; resolve the tenant admin user UUID instead.
		channelUserID := gw.resolveTenantUserIDForChannel(ctx, defaultTenant, userID)
		req := agent.RunRequest{
			AgentID: msg.AgentID, SessionID: sessionID,
			UserMessage: msg.Content, Channel: msg.ChannelType,
			SourceChannel: msg.ChannelType,
			UserID:   channelUserID,
			TenantID: defaultTenant,
		}
		enrichRunRequest(&req, msg)

		// Tag the session with the channel that originated this message
		gw.db.Pool.Exec(ctx,
			`UPDATE sessions SET source_channel = $1 WHERE id = $2`,
			msg.ChannelType, sessionID)

		// Non-interactive channels (email, webhooks): route through inbound classification pipeline.
		// Chat-family channels (telegram, whatsapp, discord, webchat) bypass this and go straight
		// to the streaming agent loop so replies are immediate and conversational.
		if gw.inbound != nil && !channels.IsInternalChannel(msg.ChannelType) && !isChatFamilyChannel(msg.ChannelType) {
			gw.inbound.Process(ctx, msg)
			return
		}

		// Telegram placeholder + streaming
		// A minimal "..." placeholder is sent so we have a message ID to edit with
		// streaming deltas. The typing indicator (sent separately by the inbound
		// handler) already signals activity — no "⏳ Thinking..." text needed.
		var placeholderID int
		var lastEdit time.Time
		var accumulated string
		if tgCh, ok := gw.findTelegramChannel(msg.AgentID); ok {
			var cid int64
			fmt.Sscanf(chatID, "%d", &cid)
			if pid, err := tgCh.SendPlaceholder(cid, "..."); err == nil {
				placeholderID = pid
			}
		}

		// Voice detection
		isVoiceMessage := msg.Metadata["media_type"] == "voice" || msg.Metadata["media_type"] == "audio"

		// Run agent loop
		result, err := gw.agentLoop.Run(ctx, req, func(event agent.StreamEvent) {
			// Broadcast to WebSocket hub
			if gw.rtHub != nil && (event.Type == "tool_start" || event.Type == "tool_result" || event.Type == "thinking_delta") {
				gw.rtHub.Broadcast(realtime.Event{
					Type: event.Type,
					Data: map[string]any{"agent_id": msg.AgentID, "session_id": sessionID, "data": event.Data},
				})
			}
			// Stream edits to Telegram placeholder
			if placeholderID > 0 && (event.Type == "text_delta" || event.Type == "stream_delta") {
				if delta, ok := event.Data.(string); ok {
					accumulated += delta
					if time.Since(lastEdit) > 1500*time.Millisecond && len(accumulated) > 0 {
						if tgCh, ok := gw.findTelegramChannel(msg.AgentID); ok {
							var cid int64
							fmt.Sscanf(chatID, "%d", &cid)
							tgCh.EditMessage(cid, placeholderID, accumulated+"▌")
							lastEdit = time.Now()
						}
					}
				}
			}
		})

		// Handle errors
		if err != nil {
			slog.Error("channel.agent_run.error", "type", msg.ChannelType, "agent", msg.AgentID, "error", err)
			if errMsg := formatAgentError(err); errMsg != "" {
				gw.sendToChannel(ctx, msg.AgentID, chatID, errMsg, "", "")
			}
			return
		}

		// Handle empty/nil result
		if result == nil || result.Content == "" {
			slog.Warn("channel.reply.empty", "agent", msg.AgentID)
			return
		}

		// Silent reply suppression
		if isSilentReply(result.Content) {
			slog.Info("channel.silent", "agent", msg.AgentID)
			return
		}

		// Deliver response
		slog.Info("channel.reply", "type", msg.ChannelType, "chat_id", chatID, "content_len", len(result.Content))

		// Try editing Telegram placeholder first
		if placeholderID > 0 {
			if tgCh, ok := gw.findTelegramChannel(msg.AgentID); ok {
				var cid int64
				fmt.Sscanf(chatID, "%d", &cid)
				// Cancel typing before editing — Send() won't run on this path.
				tgCh.StopTyping(cid)
				if err := tgCh.EditMessage(cid, placeholderID, result.Content); err == nil {
					goto voiceCheck
				}
			}
		}

		// Send reply back on the SAME channel type the message came from.
		// Pass inbound metadata so channels (e.g. Discord) can edit/delete
		// their placeholder messages using placeholder_key.
		gw.sendToChannelByTypeWithMeta(ctx, msg.AgentID, msg.ChannelType, chatID, result.Content, msg.Subject, msg.Metadata["message_id"], mediaFromResult(result), msg.Metadata)
		// NOTE: assistant reply is broadcast to WS by agentLoop.OnMessage (loop_helpers.go).
		// Do NOT broadcast again here — that causes duplicate bubbles in webchat.

	voiceCheck:
		// Auto-TTS for voice messages
		if isVoiceMessage && result.Content != "" && gw.voicePipeline != nil && gw.voicePipeline.CanSynthesize() {
			go func() {
				audioResult, err := gw.voicePipeline.SynthesizeSpeech(context.Background(), result.Content, msg.ChannelType, "")
				if err != nil {
					slog.Warn("voice.auto_tts.failed", "channel", msg.ChannelType, "error", err)
					return
				}
				if tgCh, ok := gw.findTelegramChannel(msg.AgentID); ok {
					var cid int64
					fmt.Sscanf(chatID, "%d", &cid)
					tmpWav := fmt.Sprintf("/tmp/qorven-voice-%d.wav", time.Now().UnixNano())
					tmpOgg := fmt.Sprintf("/tmp/qorven-voice-%d.ogg", time.Now().UnixNano())
					if err := os.WriteFile(tmpWav, audioResult.Audio, 0644); err == nil {
						convertCmd := exec.CommandContext(context.Background(), "ffmpeg", "-y", "-i", tmpWav, "-c:a", "libopus", "-b:a", "48k", tmpOgg)
						if convertErr := convertCmd.Run(); convertErr == nil {
							tgCh.SendVoice(context.Background(), cid, tmpOgg)
						} else {
							tgCh.SendVoice(context.Background(), cid, tmpWav)
						}
						os.Remove(tmpWav)
						os.Remove(tmpOgg)
					}
				}
			}()
		}

		slog.Info("channel.processed", "type", msg.ChannelType, "from", msg.SenderName, "agent", msg.AgentID)
	})

	// ─── Voice system ──────────────────────────────────────────────────
	//
	// Manager starts with two always-available free defaults (Edge
	// neural TTS, local faster-whisper STT) so a fresh install that
	// never visits Settings → Voice still has a working round-trip.
	// Anything beyond that is picked up from the voice_providers DB
	// table — the Settings UI and the setup wizard POST rows there.
	gw.voiceMgr = voice.NewManager()
	gw.voiceMgr.RegisterTTS(voice.NewEdgeTTS("en-US-AriaNeural"))
	gw.voiceMgr.RegisterSTT(voice.NewFasterWhisperSTT("", "base"))

	// DB-driven provider registration. Failures are logged and don't
	// block boot — one broken row shouldn't take the gateway down.
	if gw.voiceStore != nil {
		rows, err := gw.voiceStore.List(context.Background(), defaultTenant)
		if err != nil {
			slog.Warn("voice.store.list_failed", "error", err)
		} else {
			for _, r := range rows {
				if !r.Enabled {
					continue
				}
				tts, stt, err := voice.BuildProvider(r)
				if err != nil {
					slog.Warn("voice.provider.build_failed",
						"id", r.ID, "driver", r.Driver, "kind", r.Kind, "error", err)
					continue
				}
				if tts != nil {
					gw.voiceMgr.RegisterTTS(tts)
				}
				if stt != nil {
					gw.voiceMgr.RegisterSTT(stt)
				}
				if r.IsDefault {
					switch r.Kind {
					case "tts":
						if tts != nil {
							gw.voiceMgr.SetPrimaryTTS(tts.Name())
						}
					case "stt":
						if stt != nil {
							gw.voiceMgr.SetPrimarySTT(stt.Name())
						}
					}
				}
				slog.Info("voice.provider.loaded",
					"name", r.Name, "driver", r.Driver, "kind", r.Kind,
					"is_default", r.IsDefault)
			}
		}
	}

	// Voice pipeline (auto-transcribe + auto-TTS for channels)
	if gw.agentLoop != nil {
		gw.voicePipeline = voice.NewVoicePipeline(gw.voiceMgr, func(ctx context.Context, agentID, sessionID, message string) (string, error) {
			result, err := gw.agentLoop.Run(ctx, agent.RunRequest{
				AgentID: agentID, SessionID: sessionID, UserMessage: message, Channel: "voice",
			}, func(event agent.StreamEvent) {})
			if err != nil {
				return "", err
			}
			return result.Content, nil
		})
		slog.Info("voice pipeline initialized", "has_tts", gw.voiceMgr.HasTTS(), "has_stt", gw.voiceMgr.HasSTT())
	}

	// ─── Media generation system (image / video) ───────────────────────
	gw.mediaMgr = mediagen.NewManager()
	if gw.mediaStore != nil {
		rows, err := gw.mediaStore.List(context.Background(), defaultTenant)
		if err != nil {
			slog.Warn("mediagen.store.list_failed", "error", err)
		} else {
			fallbacks := []string{}
			for _, r := range rows {
				if !r.Enabled {
					continue
				}
				switch r.Kind {
				case "image":
					p, err := mediagen.BuildProvider(r)
					if err != nil {
						slog.Warn("mediagen.provider.build_failed", "id", r.ID, "driver", r.Driver, "error", err)
						continue
					}
					gw.mediaMgr.RegisterImage(p)
					if r.IsDefault {
						gw.mediaMgr.SetPrimaryImage(p.Name())
					} else {
						fallbacks = append(fallbacks, p.Name())
					}
				case "video":
					vp, err := mediagen.BuildVideoProvider(r)
					if err != nil {
						slog.Warn("mediagen.video_provider.build_failed", "id", r.ID, "driver", r.Driver, "error", err)
						continue
					}
					gw.mediaMgr.RegisterVideo(vp)
					if r.IsDefault {
						gw.mediaMgr.SetPrimaryVideo(vp.Name())
					}
				}
				slog.Info("mediagen.provider.loaded", "name", r.Name, "kind", r.Kind, "driver", r.Driver)
			}
			if len(fallbacks) > 0 {
				gw.mediaMgr.SetFallbackImage(fallbacks)
			}
		}
	}

	// Load SSRF allowlist from config
	if len(cfg.Server.SSRFAllowedHosts) > 0 {
		SetSSRFAllowlist(cfg.Server.SSRFAllowedHosts)
	}

	// IMPORTANT: register app-level middleware + routes BEFORE loading
	// channels. chi panics if Use() is called on a mux that already has
	// routes, and loadChannels() calls gw.router.Post(webhookPath, …)
	// which counts as a route. Historically loadChannels was called
	// first (so webhooks shared the mux with later routes) but that
	// only worked when no channel had a webhook handler to wire.
	gw.registerRoutes()

	// Core 1 API routes
	gw.router.Route("/api", func(api chi.Router) {
		api.Use(gw.AuthMiddlewareV2)
		gw.RegisterCore1Routes(api)
	})

	// Now that middleware + root routes are in place, attach channel
	// webhooks. Channel webhooks sit alongside the other Mux routes
	// and inherit the same global middleware stack.
	if gw.agentLoop != nil {
		gw.loadChannels()
	}
	return gw, nil
}

// dispatchRuntimeSignal is the RuntimeRunFn called by every AgentRuntime (071).
// resolveTenantUserIDForChannel returns a valid UUID user ID for the permission gate.
// Channel sessions (Telegram, WhatsApp, etc.) carry a non-UUID sender ID; we fall back
// to the tenant's first active admin user so the gate's HasPolicyForAgent query succeeds.
// Results are not cached because this is called once per inbound message and the DB round
// trip is negligible compared to the LLM latency.
func (gw *Gateway) resolveTenantUserIDForChannel(ctx context.Context, tenantID, senderID string) string {
	// If senderID is already a UUID (web sessions set it from the JWT), use it directly.
	if _, err := uuid.Parse(senderID); err == nil {
		return senderID
	}
	if gw.db == nil || tenantID == "" {
		return ""
	}
	var userID string
	gw.db.Pool.QueryRow(ctx,
		`SELECT id::text FROM users WHERE tenant_id = $1::uuid AND is_active = true ORDER BY created_at LIMIT 1`,
		tenantID,
	).Scan(&userID) //nolint:errcheck
	return userID
}

func (gw *Gateway) dispatchRuntimeSignal(ctx context.Context, agentID string, sig agent.WakeupSignal) {
	switch sig.Source {
	case agent.WakeupAssignment, agent.WakeupSynthesis:
		if sig.TaskID == "" {
			slog.Warn("dispatch.no_task_id", "agent", agentID, "source", sig.Source)
			return
		}
		gw.processTask(ctx, agentID, sig.TaskID)

	case agent.WakeupChannelMessage:
		msg, _ := sig.Context["message"].(string)
		sessID, _ := sig.Context["session_id"].(string)
		if msg == "" {
			return
		}
		req := agent.RunRequest{
			AgentID:     agentID,
			SessionID:   sessID,
			UserMessage: msg,
			Channel:     "task",
		}
		_, _ = gw.agentLoop.Run(ctx, req, func(_ agent.StreamEvent) {}) //nolint:errcheck

	case agent.WakeupManual:
		msg := sig.Message
		if msg == "" {
			msg = "You have been manually woken. Check for pending work and act on it."
		}
		req := agent.RunRequest{
			AgentID:     agentID,
			UserMessage: msg,
			Channel:     "task",
		}
		_, _ = gw.agentLoop.Run(ctx, req, func(_ agent.StreamEvent) {}) //nolint:errcheck

	default:
		slog.Warn("dispatch.unknown_source", "agent", agentID, "source", sig.Source)
	}
}
