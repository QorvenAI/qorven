-- Qorven complete database schema
-- Generated from running DB — this IS the schema, not a diff.
-- Run once on fresh install → version 1.
--
-- DO NOT APPEND TO THIS FILE.
-- New columns/tables → create backend/migrations/002_your_change.up.sql

--
--

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;

--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: -
--

--
-- Name: vector; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

--
-- Name: EXTENSION vector; Type: COMMENT; Schema: -; Owner: -
--

--
-- Name: app_current_tenant(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.app_current_tenant() RETURNS uuid
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    raw TEXT := current_setting('app.current_tenant_id', true);
BEGIN
    IF raw IS NULL OR raw = '' THEN
        RETURN NULL;
    END IF;
    RETURN raw::UUID;
EXCEPTION WHEN invalid_text_representation THEN
    -- Malformed GUC → treat as unset (deny).
    RETURN NULL;
END $$;

--
-- Name: FUNCTION app_current_tenant(); Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON FUNCTION public.app_current_tenant() IS 'Parses the app.current_tenant_id GUC to UUID. NULL on missing or malformed, which the RLS policies interpret as deny. Phase 4 migration 040.';

--
-- Name: app_rls_bypass(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.app_rls_bypass() RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
BEGIN
    -- current_setting(..., true) returns '' if the GUC isn't set,
    -- which is the secure default.
    RETURN current_setting('app.bypass_rls', true) = 'on';
END $$;

--
-- Name: FUNCTION app_rls_bypass(); Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON FUNCTION public.app_rls_bypass() IS 'Returns TRUE when app.bypass_rls GUC is ''on''. Phase 4 migration 040 introduced this; Phase 5 migration 042 extended its reach to legacy policies from migrations 001/003/004.';

--
-- Name: uuid_generate_v7(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE OR REPLACE FUNCTION public.uuid_generate_v7() RETURNS uuid
    LANGUAGE plpgsql
    AS $$
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
END $$;

--
-- Name: agent_bundles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.agent_bundles (
    id text DEFAULT (gen_random_uuid())::text NOT NULL,
    agent_id text NOT NULL,
    bundle_type text NOT NULL,
    name text NOT NULL,
    content text NOT NULL,
    priority integer DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: agent_channel_bindings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.agent_channel_bindings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    channel_type text NOT NULL,
    instance_id text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    credentials jsonb DEFAULT '{}'::jsonb NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: agent_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.agent_messages (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    from_agent uuid NOT NULL,
    to_agent uuid NOT NULL,
    task_id uuid,
    content text NOT NULL,
    message_type character varying(20) DEFAULT 'message'::character varying NOT NULL,
    read boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT agent_messages_message_type_check CHECK (((message_type)::text = ANY (ARRAY[('message'::character varying)::text, ('delegation'::character varying)::text, ('report'::character varying)::text, ('escalation'::character varying)::text, ('review'::character varying)::text])))
);

ALTER TABLE ONLY public.agent_messages FORCE ROW LEVEL SECURITY;

--
-- Name: agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.agents (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_key character varying(100) NOT NULL,
    display_name character varying(255),
    provider_id uuid,
    model character varying(200) DEFAULT 'default'::character varying NOT NULL,
    system_prompt text DEFAULT ''::text NOT NULL,
    context_window integer DEFAULT 128000 NOT NULL,
    max_tool_iterations integer DEFAULT 20 NOT NULL,
    tools_allowed jsonb,
    tools_denied jsonb,
    memory_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    other_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    credit_budget_cents bigint DEFAULT 0,
    credit_used_cents bigint DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    deleted_at timestamp with time zone,
    outbound_approval text DEFAULT 'supervisor'::text,
    role character varying(100),
    title character varying(255),
    manager_id uuid,
    avatar character varying(255),
    temperature real DEFAULT 0.7,
    tool_profile character varying(20) DEFAULT 'full'::character varying,
    skills text[] DEFAULT '{}'::text[],
    memory_enabled boolean DEFAULT true,
    memory_sharing character varying(20) DEFAULT 'private'::character varying,
    auto_compact boolean DEFAULT true,
    web_search_enabled boolean DEFAULT true,
    permissions text[] DEFAULT '{}'::text[],
    mail_approval_policy jsonb DEFAULT '{"approval_mode": "require", "auto_approve_replies": false, "approval_expiry_hours": 24}'::jsonb,
    drive_quota_bytes bigint DEFAULT 104857600,
    monthly_budget_usd numeric(10,2) DEFAULT 0,
    daily_budget_usd numeric(10,2) DEFAULT 0,
    on_limit_action text DEFAULT 'pause'::text,
    fallback_model character varying(255) DEFAULT ''::character varying,
    dreaming_enabled boolean DEFAULT true,
    dreaming_interval_hours integer DEFAULT 6,
    dreaming_mode character varying(20) DEFAULT 'balanced'::character varying,
    last_dream_at timestamp with time zone,
    next_dream_at timestamp with time zone,
    project_brief_id uuid,
    thinking_level text DEFAULT 'off'::text NOT NULL,
    runtime_mode text DEFAULT 'oneshot'::text NOT NULL,
    can_delegate boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY public.agents FORCE ROW LEVEL SECURITY;

--
-- Name: COLUMN agents.outbound_approval; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.agents.outbound_approval IS 'Approval mode: none, supervisor, user, both';

--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.api_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    name character varying(255) DEFAULT 'default'::character varying,
    key_hash text NOT NULL,
    usage_count integer DEFAULT 0,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: app_schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.app_schema_migrations (
    app_slug text NOT NULL,
    tenant_id uuid NOT NULL,
    version integer NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: approval_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.approval_comments (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    approval_id uuid NOT NULL,
    author text NOT NULL,
    author_is text DEFAULT 'user'::text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approval_comments_author_is_chk CHECK ((author_is = ANY (ARRAY['user'::text, 'agent'::text, 'system'::text])))
);

--
-- Name: approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.approvals (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    plan_id uuid NOT NULL,
    node_id uuid NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    requested_by text DEFAULT ''::text NOT NULL,
    resolved_by text,
    resolved_at timestamp with time zone,
    budget jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT approvals_state_chk CHECK ((state = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'revision_requested'::text])))
);

ALTER TABLE ONLY public.approvals FORCE ROW LEVEL SECURITY;

--
-- Name: apps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.apps (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    slug text NOT NULL,
    display_name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    version text DEFAULT '0.0.0'::text NOT NULL,
    author text DEFAULT ''::text NOT NULL,
    icon_url text DEFAULT ''::text NOT NULL,
    install_path text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    installed_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    scope text DEFAULT 'workspace'::text NOT NULL,
    owner_agent_id text,
    owner_team_id text,
    CONSTRAINT apps_scope_check CHECK ((scope = ANY (ARRAY['workspace'::text, 'agent'::text, 'team'::text]))),
    CONSTRAINT apps_slug_shape CHECK ((slug ~ '^[a-z][a-z0-9\-]{0,62}$'::text))
);

ALTER TABLE ONLY public.apps FORCE ROW LEVEL SECURITY;

--
-- Name: audit_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.audit_log (
    id bigint NOT NULL,
    tenant_id text NOT NULL,
    actor_type text NOT NULL,
    actor_id text NOT NULL,
    actor_name text DEFAULT ''::text NOT NULL,
    action text NOT NULL,
    resource text NOT NULL,
    resource_id text DEFAULT ''::text NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    ip_address text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: audit_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE IF NOT EXISTS public.audit_log_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: audit_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.audit_log_id_seq OWNED BY public.audit_log.id;

--
-- Name: brief_agent_spend; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.brief_agent_spend (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    brief_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    cost_cents numeric(10,4) DEFAULT 0 NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: builtin_tools; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.builtin_tools (
    name text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    category text DEFAULT 'general'::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    requires jsonb DEFAULT '[]'::jsonb NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: TABLE builtin_tools; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.builtin_tools IS 'Platform-global built-in tool catalog. Seeded at startup by gateway/builtin_tools.go. Operators can toggle enabled and override settings per-row. Per-tenant overrides live in builtin_tool_tenant_configs (future).';

--
-- Name: calendar_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.calendar_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    title text NOT NULL,
    description text,
    start_at timestamp with time zone NOT NULL,
    end_at timestamp with time zone,
    all_day boolean DEFAULT false,
    color text DEFAULT 'violet'::text,
    location text,
    recurrence text,
    event_type text DEFAULT 'event'::text NOT NULL,
    source_id uuid,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: category_model_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.category_model_assignments (
    tenant_id uuid NOT NULL,
    category_slug text NOT NULL,
    model_id text NOT NULL,
    priority integer DEFAULT 0
);

--
-- Name: channel_instances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.channel_instances (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    channel_type character varying(50) NOT NULL,
    credentials bytea,
    config jsonb DEFAULT '{}'::jsonb,
    enabled boolean DEFAULT true,
    status text DEFAULT 'disconnected'::text,
    last_error text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    dm_policy text DEFAULT 'pairing'::text,
    group_policy text DEFAULT 'open'::text,
    allowlist text[] DEFAULT '{}'::text[]
);

ALTER TABLE ONLY public.channel_instances FORCE ROW LEVEL SECURITY;

--
-- Name: config_secrets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.config_secrets (
    tenant_id uuid NOT NULL,
    key character varying(100) NOT NULL,
    value bytea NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.config_secrets FORCE ROW LEVEL SECURITY;

--
-- Name: connector_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.connector_actions (
    id text DEFAULT (gen_random_uuid())::text NOT NULL,
    platform_id text NOT NULL,
    action_key text NOT NULL,
    name text NOT NULL,
    description text NOT NULL,
    when_to_use text NOT NULL,
    method text NOT NULL,
    path text NOT NULL,
    headers jsonb DEFAULT '{}'::jsonb NOT NULL,
    params jsonb DEFAULT '{}'::jsonb NOT NULL,
    body_template text DEFAULT ''::text NOT NULL,
    response_desc text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: connector_credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.connector_credentials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    connector_id text NOT NULL,
    credentials jsonb DEFAULT '{}'::jsonb NOT NULL,
    status text DEFAULT 'active'::text,
    last_tested_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: connector_platforms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.connector_platforms (
    id text NOT NULL,
    name text NOT NULL,
    category text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    icon text DEFAULT ''::text NOT NULL,
    auth_type text NOT NULL,
    auth_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    base_url text NOT NULL,
    docs_url text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: connector_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.connector_snapshots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    source_slug text NOT NULL,
    result_key text DEFAULT 'data'::text NOT NULL,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: contacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.contacts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    external_id text NOT NULL,
    channel text NOT NULL,
    display_name text,
    notes text,
    pipeline_stage text DEFAULT 'lead'::text NOT NULL,
    tags text[] DEFAULT '{}'::text[] NOT NULL,
    first_seen timestamp with time zone DEFAULT now() NOT NULL,
    last_seen timestamp with time zone DEFAULT now() NOT NULL,
    message_count bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: cost_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.cost_events (
    id bigint NOT NULL,
    tenant_id text NOT NULL,
    agent_id text NOT NULL,
    session_id text DEFAULT ''::text NOT NULL,
    provider text NOT NULL,
    model text NOT NULL,
    input_tokens integer DEFAULT 0 NOT NULL,
    output_tokens integer DEFAULT 0 NOT NULL,
    cost_cents numeric(12,4) DEFAULT 0 NOT NULL,
    tool_name text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    trace_id text DEFAULT ''::text NOT NULL,
    latency_ms integer DEFAULT 0 NOT NULL,
    cache_read_tokens integer DEFAULT 0 NOT NULL,
    cache_write_tokens integer DEFAULT 0 NOT NULL
);

--
-- Name: cost_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE IF NOT EXISTS public.cost_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: cost_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cost_events_id_seq OWNED BY public.cost_events.id;

--
-- Name: credentials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.credentials (
    id text DEFAULT (gen_random_uuid())::text NOT NULL,
    tenant_id text NOT NULL,
    platform_id text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    auth_type text NOT NULL,
    data_encrypted bytea NOT NULL,
    scopes text[] DEFAULT '{}'::text[] NOT NULL,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: crew_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.crew_members (
    crew_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    role_in_crew character varying(50) DEFAULT 'specialist'::character varying,
    "position" integer DEFAULT 0
);

--
-- Name: crews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.crews (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    pattern character varying(20) DEFAULT 'supervisor'::character varying NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    supervisor_id uuid,
    status character varying(20) DEFAULT 'active'::character varying,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT crews_pattern_check CHECK (((pattern)::text = ANY (ARRAY[('single'::character varying)::text, ('supervisor'::character varying)::text, ('router'::character varying)::text, ('handoff'::character varying)::text, ('pipeline'::character varying)::text, ('blackboard'::character varying)::text])))
);

--
-- Name: cron_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.cron_jobs (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    name character varying(255) NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    cron_expression character varying(100),
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(20),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.cron_jobs FORCE ROW LEVEL SECURITY;

--
-- Name: crystallized_skills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.crystallized_skills (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    procedure text NOT NULL,
    scope text DEFAULT 'private'::text,
    mode text DEFAULT 'CAPTURED'::text,
    parent_id uuid,
    reuse_count integer DEFAULT 0,
    success_rate double precision DEFAULT 1.0,
    token_saved integer DEFAULT 0,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: custom_tools; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.custom_tools (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    command text NOT NULL,
    timeout_seconds integer DEFAULT 60 NOT NULL,
    env bytea,
    agent_id uuid,
    enabled boolean DEFAULT true NOT NULL,
    created_by character varying(255),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: daemon_agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.daemon_agents (
    id text NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    provider text NOT NULL,
    model text DEFAULT ''::text NOT NULL,
    capabilities text[] DEFAULT '{}'::text[] NOT NULL,
    status text DEFAULT 'idle'::text NOT NULL,
    current_task_id text,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    registered_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: daemon_plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.daemon_plans (
    id text NOT NULL,
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    proposed_by text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    modifications text,
    reject_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    tasks jsonb DEFAULT '[]'::jsonb NOT NULL,
    decided_at timestamp with time zone,
    decided_by text
);

--
-- Name: daemon_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.daemon_tasks (
    id text NOT NULL,
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    owner text,
    priority text DEFAULT 'normal'::text NOT NULL,
    status text DEFAULT 'queued'::text NOT NULL,
    depends_on text[] DEFAULT '{}'::text[] NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    plan_id text,
    files_changed text[] DEFAULT '{}'::text[] NOT NULL,
    percent smallint DEFAULT 0 NOT NULL,
    summary text,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);

--
-- Name: deployment_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.deployment_config (
    key text NOT NULL,
    value text NOT NULL,
    description text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: discussions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.discussions (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    ai_label text DEFAULT ''::text NOT NULL,
    user_label text,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    last_active_at timestamp with time zone DEFAULT now() NOT NULL,
    message_count integer DEFAULT 0 NOT NULL
);

--
-- Name: document_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.document_chunks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    source_type text DEFAULT 'upload'::text NOT NULL,
    source_id text,
    source_name text NOT NULL,
    chunk_index integer DEFAULT 0 NOT NULL,
    content text NOT NULL,
    token_count integer DEFAULT 0 NOT NULL,
    embedding public.vector(384),
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: draft_replies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.draft_replies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    session_id uuid,
    sender_id text NOT NULL,
    sender_name text,
    channel text NOT NULL,
    original_message text NOT NULL,
    history_summary text,
    draft_content text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    approval_msg_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    decided_at timestamp with time zone,
    decided_by text
);

--
-- Name: drive_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.drive_files (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    name text NOT NULL,
    path text NOT NULL,
    mime_type text,
    size_bytes bigint DEFAULT 0,
    is_folder boolean DEFAULT false,
    parent_id uuid,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    enrichment_status character varying(20) DEFAULT 'pending'::character varying,
    summary text,
    keywords jsonb DEFAULT '[]'::jsonb,
    entities_extracted jsonb DEFAULT '[]'::jsonb
);

--
-- Name: drive_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.drive_permissions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    file_id uuid NOT NULL,
    grantee_type text NOT NULL,
    grantee_id uuid,
    permission text DEFAULT 'viewer'::text NOT NULL,
    granted_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: email_routing; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.email_routing (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    shared_mailbox text NOT NULL,
    alias text NOT NULL,
    agent_id uuid NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: evaluations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.evaluations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    session_id uuid,
    evaluator_id uuid,
    score double precision,
    criteria jsonb DEFAULT '{}'::jsonb,
    notes text,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: feedback; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.feedback (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    session_id uuid,
    agent_id uuid,
    message_id text,
    rating integer,
    comment text,
    created_at timestamp with time zone DEFAULT now(),
    model text,
    tools_used text[],
    response_tokens integer,
    latency_ms integer,
    context_tokens integer
);

--
-- Name: github_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.github_connections (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id text DEFAULT 'default'::text NOT NULL,
    agent_id text NOT NULL,
    owner text NOT NULL,
    repo text NOT NULL,
    encrypted_token bytea NOT NULL,
    webhook_secret text,
    default_branch text DEFAULT 'main'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: github_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.github_events (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id text DEFAULT 'default'::text NOT NULL,
    connection_id uuid,
    event_type text NOT NULL,
    action text,
    payload jsonb,
    session_id text,
    processed boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: goals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.goals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    title text NOT NULL,
    description text,
    status text DEFAULT 'active'::text,
    due_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    target_value double precision,
    current_value double precision,
    unit text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: heartbeat_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.heartbeat_configs (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    interval_sec integer DEFAULT 900 NOT NULL,
    model character varying(200),
    token_budget integer DEFAULT 1000 NOT NULL,
    max_iterations integer DEFAULT 3 NOT NULL,
    active_hours_start character varying(5),
    active_hours_end character varying(5),
    timezone character varying(50) DEFAULT 'UTC'::character varying NOT NULL,
    probes jsonb DEFAULT '[]'::jsonb NOT NULL,
    policy jsonb DEFAULT '{}'::jsonb NOT NULL,
    checklist text,
    current_state character varying(20) DEFAULT 'healthy'::character varying NOT NULL,
    consecutive_failures integer DEFAULT 0 NOT NULL,
    consecutive_passes integer DEFAULT 0 NOT NULL,
    last_run_at timestamp with time zone,
    last_status character varying(20),
    last_error text,
    next_run_at timestamp with time zone,
    run_count integer DEFAULT 0 NOT NULL,
    suppress_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT heartbeat_configs_current_state_check CHECK (((current_state)::text = ANY (ARRAY[('healthy'::character varying)::text, ('degraded'::character varying)::text, ('critical'::character varying)::text, ('recovering'::character varying)::text, ('unknown'::character varying)::text])))
);

ALTER TABLE ONLY public.heartbeat_configs FORCE ROW LEVEL SECURITY;

--
-- Name: heartbeat_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.heartbeat_queue (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    trigger text NOT NULL,
    context_type text DEFAULT 'ticket'::text NOT NULL,
    context_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    error_msg text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    run_at timestamp with time zone DEFAULT now() NOT NULL,
    finished_at timestamp with time zone,
    CONSTRAINT heartbeat_queue_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'running'::text, 'done'::text, 'failed'::text]))),
    CONSTRAINT heartbeat_queue_trigger_check CHECK ((trigger = ANY (ARRAY['ticket_assigned'::text, 'scheduled'::text])))
);

--
-- Name: heartbeat_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.heartbeat_runs (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    heartbeat_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    status character varying(20) NOT NULL,
    phase_reached integer DEFAULT 0 NOT NULL,
    probe_results jsonb,
    policy_state character varying(20),
    state_changed boolean DEFAULT false NOT NULL,
    llm_called boolean DEFAULT false NOT NULL,
    summary text,
    error text,
    input_tokens integer DEFAULT 0 NOT NULL,
    output_tokens integer DEFAULT 0 NOT NULL,
    duration_ms integer,
    ran_at timestamp with time zone DEFAULT now(),
    CONSTRAINT heartbeat_runs_status_check CHECK (((status)::text = ANY (ARRAY[('running'::character varying)::text, ('completed'::character varying)::text, ('suppressed'::character varying)::text, ('error'::character varying)::text, ('skipped'::character varying)::text])))
);

ALTER TABLE ONLY public.heartbeat_runs FORCE ROW LEVEL SECURITY;

--
-- Name: inbound_agent_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.inbound_agent_config (
    agent_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    default_mode text DEFAULT 'draft_and_approve'::text NOT NULL,
    unknown_sender_mode text DEFAULT 'context_only'::text NOT NULL,
    spam_action text DEFAULT 'drop'::text NOT NULL,
    notification_channel text,
    notification_target text,
    briefing_enabled boolean DEFAULT false NOT NULL,
    briefing_time text DEFAULT '08:00'::text NOT NULL,
    briefing_timezone text DEFAULT 'Asia/Shanghai'::text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: inbound_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.inbound_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    priority integer DEFAULT 100 NOT NULL,
    match_type text NOT NULL,
    match_value text NOT NULL,
    mode text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: key_usage_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.key_usage_log (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    key_id uuid,
    agent_id uuid,
    model text,
    tokens_in integer,
    tokens_out integer,
    latency_ms integer,
    status text,
    error_msg text,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: kg_entities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.kg_entities (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    name character varying(500) NOT NULL,
    entity_type character varying(100) NOT NULL,
    properties jsonb DEFAULT '{}'::jsonb NOT NULL,
    source text,
    confidence real DEFAULT 1.0,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    description text DEFAULT ''::text
);

ALTER TABLE ONLY public.kg_entities FORCE ROW LEVEL SECURITY;

--
-- Name: kg_relationships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.kg_relationships (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    source_id uuid NOT NULL,
    target_id uuid NOT NULL,
    rel_type character varying(100) NOT NULL,
    properties jsonb DEFAULT '{}'::jsonb NOT NULL,
    confidence real DEFAULT 1.0,
    created_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.kg_relationships FORCE ROW LEVEL SECURITY;

--
-- Name: learned_skills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.learned_skills (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    slug text NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    pattern_hash text NOT NULL,
    tool_sequence text[] DEFAULT '{}'::text[] NOT NULL,
    usage_count integer DEFAULT 0 NOT NULL,
    success_count integer DEFAULT 0 NOT NULL,
    last_used_at timestamp with time zone,
    skill_content text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: llm_stats_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.llm_stats_cache (
    key text NOT NULL,
    data jsonb NOT NULL,
    fetched_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: magic_links; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.magic_links (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    delivery text DEFAULT 'email'::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mail_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.mail_aliases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    alias_address text NOT NULL,
    target_agent_id uuid NOT NULL,
    can_send_as boolean DEFAULT true NOT NULL,
    can_receive boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mail_approval_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.mail_approval_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    message_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    reviewed_by text,
    reviewed_at timestamp with time zone,
    notes text,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mail_routing_rules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.mail_routing_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    rule_type text NOT NULL,
    pattern text NOT NULL,
    target_type text NOT NULL,
    target_id uuid,
    priority integer DEFAULT 100 NOT NULL,
    auto_reply boolean DEFAULT false NOT NULL,
    auto_reply_template text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mail_thread_assignments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.mail_thread_assignments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    thread_id text NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    assigned_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: mailbox_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.mailbox_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    identity_id uuid,
    thread_id text,
    message_id text NOT NULL,
    in_reply_to text,
    folder text DEFAULT 'inbox'::text NOT NULL,
    direction text DEFAULT 'inbound'::text NOT NULL,
    from_address text NOT NULL,
    from_name text,
    to_addresses text[] NOT NULL,
    cc_addresses text[],
    bcc_addresses text[],
    subject text,
    body_text text,
    body_html text,
    attachments jsonb,
    is_read boolean DEFAULT false NOT NULL,
    is_starred boolean DEFAULT false NOT NULL,
    labels text[],
    send_status text DEFAULT 'delivered'::text NOT NULL,
    raw_headers jsonb,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: media_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.media_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text DEFAULT 'default'::text NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    driver text NOT NULL,
    api_base text DEFAULT ''::text NOT NULL,
    api_key_enc text DEFAULT ''::text NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    fallback_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT media_providers_kind_check CHECK ((kind = ANY (ARRAY['image'::text, 'video'::text, 'audio_gen'::text])))
);

--
-- Name: memories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memories (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    user_id character varying(255),
    memory_type character varying(20) NOT NULL,
    content text NOT NULL,
    summary text,
    source text,
    source_type character varying(20) DEFAULT 'conversation'::character varying,
    importance real DEFAULT 0.5 NOT NULL,
    access_count integer DEFAULT 0 NOT NULL,
    last_accessed timestamp with time zone,
    decay_exempt boolean DEFAULT false NOT NULL,
    embedding public.vector(1536),
    tsv tsvector GENERATED ALWAYS AS ((setweight(to_tsvector('simple'::regconfig, COALESCE(content, ''::text)), 'A'::"char") || setweight(to_tsvector('simple'::regconfig, COALESCE(summary, ''::text)), 'B'::"char"))) STORED,
    tags text[],
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    task_id uuid,
    scope text DEFAULT 'agent'::text NOT NULL,
    CONSTRAINT memories_memory_type_check CHECK (((memory_type)::text = ANY (ARRAY[('fact'::character varying)::text, ('preference'::character varying)::text, ('decision'::character varying)::text, ('identity'::character varying)::text, ('event'::character varying)::text, ('observation'::character varying)::text, ('goal'::character varying)::text, ('todo'::character varying)::text])))
);

ALTER TABLE ONLY public.memories FORCE ROW LEVEL SECURITY;

--
-- Name: memory_backend_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memory_backend_config (
    tenant_id uuid NOT NULL,
    backend_name text DEFAULT 'postgresql'::text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: memory_bulletins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memory_bulletins (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    content text NOT NULL,
    memory_count integer DEFAULT 0 NOT NULL,
    generated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.memory_bulletins FORCE ROW LEVEL SECURITY;

--
-- Name: memory_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memory_chunks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id text NOT NULL,
    tenant_id text DEFAULT 'default'::text NOT NULL,
    content text NOT NULL,
    source text,
    chunk_index integer DEFAULT 0,
    embedding public.vector(1536),
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple'::regconfig, content)) STORED,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.memory_chunks FORCE ROW LEVEL SECURITY;

--
-- Name: memory_documents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memory_documents (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    user_id character varying(255),
    path character varying(500) NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    hash character varying(64) NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

ALTER TABLE ONLY public.memory_documents FORCE ROW LEVEL SECURITY;

--
-- Name: memory_edges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.memory_edges (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    source_id uuid NOT NULL,
    target_id uuid NOT NULL,
    edge_type character varying(20) NOT NULL,
    weight real DEFAULT 1.0 NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT memory_edges_edge_type_check CHECK (((edge_type)::text = ANY (ARRAY[('related_to'::character varying)::text, ('updates'::character varying)::text, ('contradicts'::character varying)::text, ('caused_by'::character varying)::text, ('part_of'::character varying)::text])))
);

ALTER TABLE ONLY public.memory_edges FORCE ROW LEVEL SECURITY;

--
-- Name: model_discoveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.model_discoveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    provider_id text NOT NULL,
    key_id uuid,
    model_id text NOT NULL,
    first_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    notified_at timestamp with time zone,
    user_action text
);

--
-- Name: model_pricing; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.model_pricing (
    model_id text NOT NULL,
    provider text,
    input_cost_per_token numeric(20,12),
    output_cost_per_token numeric(20,12),
    context_window integer,
    source text DEFAULT 'litellm'::text,
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.notifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid,
    user_id text DEFAULT 'user'::text,
    agent_id text,
    agent_key text,
    agent_name text,
    type text DEFAULT 'message'::text NOT NULL,
    title text NOT NULL,
    highlight text,
    content text,
    source text,
    source_id text,
    read boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: outbound_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.outbound_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    agent_id uuid NOT NULL,
    action_type text NOT NULL,
    payload jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    approval_mode text DEFAULT 'supervisor'::text NOT NULL,
    requested_at timestamp with time zone DEFAULT now(),
    reviewed_by text,
    reviewed_at timestamp with time zone,
    review_notes text,
    session_id text,
    expires_at timestamp with time zone DEFAULT (now() + '24:00:00'::interval)
);

--
-- Name: paired_devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.paired_devices (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    sender_id character varying(200) NOT NULL,
    channel character varying(255) NOT NULL,
    chat_id character varying(200) NOT NULL,
    paired_by character varying(100) DEFAULT 'operator'::character varying NOT NULL,
    paired_at timestamp with time zone DEFAULT now(),
    sender_name text DEFAULT ''::text
);

ALTER TABLE ONLY public.paired_devices FORCE ROW LEVEL SECURITY;

--
-- Name: pairing_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.pairing_requests (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    code character varying(8) NOT NULL,
    sender_id character varying(200) NOT NULL,
    channel character varying(255) NOT NULL,
    chat_id character varying(200) NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    pairing_code text,
    status text DEFAULT 'pending'::text,
    sender_name text,
    channel_type text
);

--
-- Name: permission_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.permission_requests (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    session_id uuid,
    plan_id uuid,
    node_id uuid,
    agent_key text,
    tool text NOT NULL,
    args jsonb DEFAULT '{}'::jsonb NOT NULL,
    reason text,
    state text DEFAULT 'pending'::text NOT NULL,
    requested_by text,
    replied_by text,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    replied_at timestamp with time zone,
    expires_at timestamp with time zone,
    tenant_id text DEFAULT 'default'::text NOT NULL,
    CONSTRAINT permission_state_chk CHECK ((state = ANY (ARRAY['pending'::text, 'allowed'::text, 'denied'::text, 'expired'::text])))
);

ALTER TABLE ONLY public.permission_requests FORCE ROW LEVEL SECURITY;

--
-- Name: pinned_tiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.pinned_tiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    source_slug text NOT NULL,
    tool_name text NOT NULL,
    tool_args jsonb DEFAULT '{}'::jsonb NOT NULL,
    widget_type text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    "position" integer DEFAULT 0 NOT NULL,
    refresh_interval_sec integer DEFAULT 300 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pinned_tiles_widget_type_check CHECK ((widget_type = ANY (ARRAY['stat-card'::text, 'data-table'::text, 'feed'::text, 'list'::text, 'chart'::text])))
);

--
-- Name: plan_edges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.plan_edges (
    plan_id uuid NOT NULL,
    from_node uuid NOT NULL,
    to_node uuid NOT NULL,
    condition text DEFAULT 'always'::text NOT NULL,
    CONSTRAINT plan_edges_condition_chk CHECK ((condition = ANY (ARRAY['always'::text, 'approved'::text, 'rejected'::text, 'revision'::text, 'on_success'::text, 'on_error'::text])))
);

ALTER TABLE ONLY public.plan_edges FORCE ROW LEVEL SECURITY;

--
-- Name: plan_nodes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.plan_nodes (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    plan_id uuid NOT NULL,
    parent_id uuid,
    kind text NOT NULL,
    title text DEFAULT ''::text NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    assignee_soul text,
    inputs jsonb DEFAULT '{}'::jsonb NOT NULL,
    artifacts jsonb DEFAULT '{}'::jsonb NOT NULL,
    error text,
    started_at timestamp with time zone,
    ended_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT plan_nodes_kind_chk CHECK ((kind = ANY (ARRAY['planner'::text, 'human_feedback'::text, 'agent_task'::text, 'review'::text, 'push'::text, 'preview'::text]))),
    CONSTRAINT plan_nodes_state_chk CHECK ((state = ANY (ARRAY['pending'::text, 'running'::text, 'done'::text, 'failed'::text, 'blocked'::text, 'cancelled'::text])))
);

ALTER TABLE ONLY public.plan_nodes FORCE ROW LEVEL SECURITY;

--
-- Name: plans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.plans (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    project_id uuid,
    session_id uuid,
    title text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    spec jsonb DEFAULT '{}'::jsonb NOT NULL,
    summary text,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    archived_at timestamp with time zone,
    CONSTRAINT plans_status_chk CHECK ((status = ANY (ARRAY['draft'::text, 'pending_approval'::text, 'approved'::text, 'rejected'::text, 'revision_requested'::text, 'running'::text, 'done'::text, 'failed'::text, 'cancelled'::text, 'archived'::text])))
);

ALTER TABLE ONLY public.plans FORCE ROW LEVEL SECURITY;

--
-- Name: prime_delegations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.prime_delegations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    prime_id uuid NOT NULL,
    specialist_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    user_query text NOT NULL,
    instructions text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    result text,
    error text,
    origin_channel text DEFAULT ''::text NOT NULL,
    origin_chat_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);

--
-- Name: project_briefs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.project_briefs (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    idea text DEFAULT ''::text NOT NULL,
    stack text DEFAULT ''::text NOT NULL,
    budget_cents integer DEFAULT 0 NOT NULL,
    timeline text DEFAULT ''::text NOT NULL,
    quality text DEFAULT 'mvp'::text NOT NULL,
    status text DEFAULT 'intake'::text NOT NULL,
    proposal jsonb,
    goal_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT project_briefs_quality_check CHECK ((quality = ANY (ARRAY['mvp'::text, 'production'::text, 'enterprise'::text]))),
    CONSTRAINT project_briefs_status_check CHECK ((status = ANY (ARRAY['intake'::text, 'proposed'::text, 'approved'::text, 'active'::text, 'done'::text, 'cancelled'::text])))
);

--
-- Name: prompt_cache_stats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.prompt_cache_stats (
    agent_id uuid NOT NULL,
    session_id text NOT NULL,
    prompt_hash text NOT NULL,
    hit_count integer DEFAULT 0 NOT NULL,
    tokens_saved integer DEFAULT 0 NOT NULL,
    last_hit_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: provider_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.provider_keys (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    provider_id text NOT NULL,
    label text,
    key_hash text NOT NULL,
    key_enc bytea NOT NULL,
    status text DEFAULT 'unverified'::text,
    verified_at timestamp with time zone,
    last_used_at timestamp with time zone,
    rate_limited_until timestamp with time zone,
    rotation_order integer DEFAULT 0,
    total_requests bigint DEFAULT 0,
    total_tokens_in bigint DEFAULT 0,
    total_tokens_out bigint DEFAULT 0,
    created_at timestamp with time zone DEFAULT now(),
    budget_usd_monthly numeric(10,2),
    budget_tokens_monthly bigint,
    spent_usd_month numeric(10,4) DEFAULT 0 NOT NULL,
    spent_tokens_month bigint DEFAULT 0 NOT NULL,
    budget_reset_at timestamp with time zone DEFAULT (date_trunc('month'::text, now()) + '1 mon'::interval) NOT NULL,
    discovery_last_run_at timestamp with time zone
);

--
-- Name: provider_pool_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.provider_pool_config (
    tenant_id uuid NOT NULL,
    provider_id text NOT NULL,
    strategy text DEFAULT 'priority'::text NOT NULL,
    failover_mode text DEFAULT 'on_exhaust'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.providers (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    display_name character varying(255),
    provider_type character varying(30) DEFAULT 'openai_compat'::character varying NOT NULL,
    api_base text,
    api_key bytea,
    enabled boolean DEFAULT true NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    capabilities jsonb DEFAULT '{}'::jsonb NOT NULL
);

ALTER TABLE ONLY public.providers FORCE ROW LEVEL SECURITY;

--
-- Name: COLUMN providers.capabilities; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.providers.capabilities IS 'Provider capability flags: {streaming, caching, thinking, vision, tools, parallel_tools}';

--
-- Name: qoros_daily_logs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.qoros_daily_logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id text NOT NULL,
    tenant_id text NOT NULL,
    log_date date NOT NULL,
    entry_time timestamp with time zone DEFAULT now() NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: qoros_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.qoros_state (
    agent_id text NOT NULL,
    tenant_id text NOT NULL,
    active boolean DEFAULT false NOT NULL,
    tick_count integer DEFAULT 0 NOT NULL,
    tick_interval_s integer DEFAULT 30 NOT NULL,
    sleeping boolean DEFAULT false NOT NULL,
    sleep_until timestamp with time zone,
    sleep_reason text,
    last_tick_at timestamp with time zone,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: refresh_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.refresh_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    user_agent text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: room_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_decisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    room_id uuid NOT NULL,
    tenant_id text NOT NULL,
    content text NOT NULL,
    decided_by text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    message_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: room_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_members (
    room_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    role text DEFAULT 'member'::text,
    joined_at timestamp with time zone DEFAULT now(),
    speaking_order integer DEFAULT 99,
    can_decide boolean DEFAULT false,
    last_active_at timestamp with time zone
);

--
-- Name: room_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    room_id uuid,
    sender_id text NOT NULL,
    sender_type text DEFAULT 'user'::text,
    content text NOT NULL,
    message_type text DEFAULT 'text'::text,
    metadata jsonb DEFAULT '{}'::jsonb,
    reactions jsonb DEFAULT '{}'::jsonb,
    reply_to uuid,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: room_minutes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_minutes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    room_id uuid NOT NULL,
    tenant_id text NOT NULL,
    summary text NOT NULL,
    decisions jsonb DEFAULT '[]'::jsonb NOT NULL,
    action_items jsonb DEFAULT '[]'::jsonb NOT NULL,
    blockers jsonb DEFAULT '[]'::jsonb NOT NULL,
    from_msg_id uuid,
    to_msg_id uuid,
    msg_count integer DEFAULT 0 NOT NULL,
    generated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: room_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    room_id uuid NOT NULL,
    tenant_id text NOT NULL,
    title text NOT NULL,
    description text,
    assigned_by text NOT NULL,
    assigned_to text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    due_at timestamp with time zone,
    message_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: room_typing; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.room_typing (
    room_id uuid NOT NULL,
    agent_id text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: rooms; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.rooms (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    display_name text DEFAULT ''::text,
    description text DEFAULT ''::text,
    created_by text DEFAULT 'user'::text,
    is_dm boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: routing_decisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.routing_decisions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    soul_id uuid,
    query_preview text,
    classified_category text NOT NULL,
    confidence double precision,
    routed_model text NOT NULL,
    user_override_model text,
    user_override_category text,
    was_correct boolean,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: running_apps; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.running_apps (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    session_id uuid,
    agent_id uuid,
    container_id text NOT NULL,
    image text NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    proxy_prefix text NOT NULL,
    internal_port integer NOT NULL,
    host_port integer NOT NULL,
    status text DEFAULT 'running'::text NOT NULL,
    env jsonb DEFAULT '{}'::jsonb NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT running_apps_status_check CHECK ((status = ANY (ARRAY['running'::text, 'stopped'::text, 'error'::text])))
);

--
-- Name: sandbox_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.sandbox_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid,
    command text NOT NULL,
    language text,
    code text,
    exit_code integer,
    output text,
    duration_ms integer,
    status text DEFAULT 'running'::text,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: scenarios; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.scenarios (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    name text,
    seed text NOT NULL,
    agent_count integer DEFAULT 5,
    rounds integer DEFAULT 5,
    status text DEFAULT 'created'::text,
    report text,
    rounds_data jsonb,
    created_at timestamp with time zone DEFAULT now(),
    completed_at timestamp with time zone
);

--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version integer NOT NULL,
    dirty boolean DEFAULT false NOT NULL,
    applied_at timestamp with time zone DEFAULT now()
);

--
-- Name: schema_version; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.schema_version (
    version integer NOT NULL
);

--
-- Name: selected_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.selected_models (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    provider_id text NOT NULL,
    model_id text NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    category text
);

--
-- Name: service_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.service_accounts (
    id text NOT NULL,
    role text DEFAULT 'service'::text NOT NULL,
    description text,
    created_by text DEFAULT 'system'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    tenant_id uuid,
    CONSTRAINT service_accounts_role_chk CHECK ((role = ANY (ARRAY['admin'::text, 'service'::text, 'orchestrator'::text])))
);

ALTER TABLE ONLY public.service_accounts FORCE ROW LEVEL SECURITY;

--
-- Name: COLUMN service_accounts.tenant_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.service_accounts.tenant_id IS 'Tenant binding for this service account. NULL = global/infra actor (legacy: system/orchestrator/qoros). Non-NULL = tenant-scoped; authorize() must match resource tenant_id before granting bypass.';

--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.sessions (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid NOT NULL,
    session_key character varying(500) NOT NULL,
    user_id character varying(255),
    messages jsonb DEFAULT '[]'::jsonb NOT NULL,
    summary text,
    channel character varying(50),
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    label character varying(500),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    status character varying(20) DEFAULT 'active'::character varying,
    files_changed jsonb DEFAULT '[]'::jsonb,
    owner_actor_id text,
    discussion_id uuid,
    source_channel text DEFAULT 'web'::text NOT NULL,
    delivered_channels text[] DEFAULT '{web,tui}'::text[] NOT NULL
);

ALTER TABLE ONLY public.sessions FORCE ROW LEVEL SECURITY;

--
-- Name: skill_installations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.skill_installations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    manifest_id text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    install_path text,
    pid integer,
    tools_registered integer DEFAULT 0,
    error_msg text,
    installed_at timestamp with time zone DEFAULT now(),
    started_at timestamp with time zone
);

--
-- Name: skill_manifests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.skill_manifests (
    id text NOT NULL,
    name text NOT NULL,
    description text,
    repo_url text NOT NULL,
    license text,
    type text DEFAULT 'mcp'::text NOT NULL,
    transport text DEFAULT 'stdio'::text,
    install_cmd text,
    start_cmd text,
    mcp_config jsonb DEFAULT '{}'::jsonb,
    tools text[],
    tags text[],
    icon text,
    author text,
    stars integer DEFAULT 0,
    verified boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: skill_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.skill_reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    skill_id uuid NOT NULL,
    rating integer,
    review text,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT skill_reviews_rating_check CHECK (((rating >= 1) AND (rating <= 5)))
);

--
-- Name: skills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.skills (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(255) NOT NULL,
    description text,
    file_path text NOT NULL,
    file_hash character varying(64),
    tags text[],
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    version text DEFAULT '1.0.0'::text,
    author text DEFAULT ''::text,
    category text DEFAULT ''::text,
    requires_tools text[],
    requires_keys text[],
    source_url text DEFAULT ''::text,
    skill_md text DEFAULT ''::text,
    install_count integer DEFAULT 0,
    rating double precision DEFAULT 0,
    pinned boolean DEFAULT false NOT NULL
);

ALTER TABLE ONLY public.skills FORCE ROW LEVEL SECURITY;

--
-- Name: COLUMN skills.pinned; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.skills.pinned IS 'When true, skill_manage tool cannot modify or delete this skill; admin-only via API.';

--
-- Name: social_autoposts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.social_autoposts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    agent_id uuid,
    name text NOT NULL,
    source text NOT NULL,
    source_url text,
    platforms jsonb DEFAULT '[]'::jsonb NOT NULL,
    schedule text,
    template text,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: social_integrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.social_integrations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    agent_id uuid,
    platform text NOT NULL,
    account_name text NOT NULL,
    account_id text,
    access_token text,
    refresh_token text,
    token_expiry timestamp with time zone,
    scopes jsonb DEFAULT '[]'::jsonb NOT NULL,
    active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: social_posts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.social_posts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    agent_id uuid,
    team_id text,
    content text NOT NULL,
    media_urls jsonb DEFAULT '[]'::jsonb NOT NULL,
    platforms jsonb DEFAULT '[]'::jsonb NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb NOT NULL,
    status text DEFAULT 'draft'::text NOT NULL,
    scheduled_at timestamp with time zone,
    published_at timestamp with time zone,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: soul_mail_identities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.soul_mail_identities (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    address text NOT NULL,
    display_name text NOT NULL,
    identity_type text DEFAULT 'dedicated'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    dkim_selector text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    smtp_host text DEFAULT ''::text,
    smtp_port integer DEFAULT 587,
    smtp_user text DEFAULT ''::text,
    imap_host text DEFAULT ''::text,
    imap_port integer DEFAULT 993,
    imap_user text DEFAULT ''::text,
    poll_interval_seconds integer DEFAULT 60
);

--
-- Name: soul_skills; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.soul_skills (
    agent_id text NOT NULL,
    skill_id uuid NOT NULL,
    installed_at timestamp with time zone DEFAULT now()
);

--
-- Name: soul_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.soul_usage (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    soul_id uuid NOT NULL,
    provider_key_id uuid,
    model_id text NOT NULL,
    input_tokens integer NOT NULL,
    output_tokens integer NOT NULL,
    cost_usd numeric(12,8),
    called_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: spans; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.spans (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    trace_id uuid NOT NULL,
    span_type character varying(20) NOT NULL,
    name text,
    model character varying(200),
    provider character varying(50),
    input_tokens integer,
    output_tokens integer,
    cost_cents integer DEFAULT 0,
    start_time timestamp with time zone DEFAULT now(),
    end_time timestamp with time zone,
    duration_ms integer,
    status character varying(20) DEFAULT 'running'::character varying,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.spans FORCE ROW LEVEL SECURITY;

--
-- Name: stream_checkpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.stream_checkpoints (
    session_id text NOT NULL,
    tenant_id text NOT NULL,
    last_seq bigint DEFAULT 0 NOT NULL,
    last_event_id text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: subagent_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.subagent_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    parent_agent_id uuid,
    child_agent_id uuid,
    task text NOT NULL,
    status text DEFAULT 'pending'::text,
    result text,
    error text,
    started_at timestamp with time zone,
    ended_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: supervisor_exchanges; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.supervisor_exchanges (
    id text NOT NULL,
    agent_a text NOT NULL,
    agent_b text NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    closed_at timestamp with time zone
);

--
-- Name: supervisor_fix_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.supervisor_fix_history (
    id integer NOT NULL,
    fix_type text NOT NULL,
    params jsonb DEFAULT '{}'::jsonb,
    success boolean NOT NULL,
    error text,
    duration_ms integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: supervisor_fix_history_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE IF NOT EXISTS public.supervisor_fix_history_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

--
-- Name: supervisor_fix_history_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.supervisor_fix_history_id_seq OWNED BY public.supervisor_fix_history.id;

--
-- Name: supervisor_messages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.supervisor_messages (
    id text NOT NULL,
    exchange_id text,
    from_agent text NOT NULL,
    to_agent text NOT NULL,
    intent text NOT NULL,
    content text,
    context jsonb DEFAULT '{}'::jsonb,
    risk text DEFAULT 'low'::text,
    reply_to text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: supervisor_reviews; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.supervisor_reviews (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    from_agent_id text NOT NULL,
    to_agent_id text NOT NULL,
    session_id text NOT NULL,
    content text NOT NULL,
    risk_level text DEFAULT 'medium'::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    resolved_at timestamp with time zone,
    resolution_note text
);

--
-- Name: system_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.system_configs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    key text NOT NULL,
    value jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: task_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.task_comments (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    task_id uuid NOT NULL,
    author_type text NOT NULL,
    author_id text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_comments_author_type_check CHECK ((author_type = ANY (ARRAY['user'::text, 'agent'::text])))
);

--
-- Name: task_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.task_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    task_id uuid NOT NULL,
    agent_id uuid,
    event_type text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    tokens_used integer DEFAULT 0 NOT NULL,
    cost_cents integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: task_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.task_files (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    task_id uuid NOT NULL,
    path text NOT NULL,
    operation text NOT NULL,
    touched_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT task_files_operation_check CHECK ((operation = ANY (ARRAY['created'::text, 'modified'::text, 'deleted'::text])))
);

--
-- Name: tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.tasks (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    parent_id uuid,
    title character varying(500) NOT NULL,
    description text,
    context text,
    assigned_to uuid,
    assigned_by uuid,
    status character varying(20) DEFAULT 'backlog'::character varying NOT NULL,
    priority integer DEFAULT 3 NOT NULL,
    result text,
    tokens_used bigint DEFAULT 0 NOT NULL,
    cost_cents bigint DEFAULT 0 NOT NULL,
    due_at timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    state text DEFAULT 'backlog'::text NOT NULL,
    assigned_agent_id text,
    parent_task_id text,
    locked_by text,
    locked_at timestamp with time zone,
    review_notes text DEFAULT ''::text,
    scratchpad text DEFAULT ''::text NOT NULL,
    last_heartbeat_at timestamp with time zone,
    budget_cents integer DEFAULT 0 NOT NULL,
    iteration_count integer DEFAULT 0 NOT NULL,
    synthesis_triggered_at timestamp with time zone,
    permissions jsonb DEFAULT '{"exec": true, "git_push": false, "web_fetch": true, "email_send": false, "file_delete": false}'::jsonb NOT NULL,
    allowed_domains text[] DEFAULT '{}'::text[] NOT NULL,
    discussion_id uuid,
    origin_session_id uuid,
    github_issue_number integer,
    github_pr_number integer,
    github_pr_url text,
    github_branch text,
    ticket_id uuid,
    CONSTRAINT tasks_priority_check CHECK (((priority >= 1) AND (priority <= 5))),
    CONSTRAINT tasks_status_check CHECK (((status)::text = ANY (ARRAY[('backlog'::character varying)::text, ('assigned'::character varying)::text, ('in_progress'::character varying)::text, ('review'::character varying)::text, ('done'::character varying)::text, ('blocked'::character varying)::text, ('cancelled'::character varying)::text])))
);

ALTER TABLE ONLY public.tasks FORCE ROW LEVEL SECURITY;

--
-- Name: team_task_attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.team_task_attachments (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    task_id uuid NOT NULL,
    team_id uuid NOT NULL,
    chat_id text,
    path text NOT NULL,
    file_size bigint DEFAULT 0,
    mime_type text,
    created_by_agent_id uuid,
    created_by_sender_id text,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: team_task_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.team_task_comments (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    task_id uuid NOT NULL,
    agent_id uuid,
    user_id text,
    content text NOT NULL,
    comment_type character varying(20) DEFAULT 'note'::character varying,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: team_task_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.team_task_events (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    task_id uuid NOT NULL,
    event_type character varying(50) NOT NULL,
    actor_type character varying(20) NOT NULL,
    actor_id text,
    data jsonb,
    created_at timestamp with time zone DEFAULT now()
);

--
-- Name: tenant_skill_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.tenant_skill_config (
    tenant_id uuid NOT NULL,
    skill_slug text NOT NULL,
    enabled boolean DEFAULT true,
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: tenants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.tenants (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(100) NOT NULL,
    plan character varying(50) DEFAULT 'free'::character varying NOT NULL,
    credit_budget_cents bigint DEFAULT 0 NOT NULL,
    credit_used_cents bigint DEFAULT 0 NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: ticket_comments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.ticket_comments (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    ticket_id uuid NOT NULL,
    author_type text NOT NULL,
    author_id text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ticket_comments_author_type_check CHECK ((author_type = ANY (ARRAY['user'::text, 'agent'::text])))
);

--
-- Name: ticket_counters; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.ticket_counters (
    tenant_id uuid NOT NULL,
    next_val bigint DEFAULT 1 NOT NULL
);

--
-- Name: ticket_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.ticket_files (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    ticket_id uuid NOT NULL,
    path text NOT NULL,
    operation text NOT NULL,
    touched_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ticket_files_operation_check CHECK ((operation = ANY (ARRAY['created'::text, 'modified'::text, 'deleted'::text])))
);

--
-- Name: tickets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.tickets (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    slug text NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'todo'::text NOT NULL,
    priority text DEFAULT 'normal'::text NOT NULL,
    assigned_agent_id uuid,
    goal_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    blocked_by uuid[] DEFAULT '{}'::uuid[] NOT NULL,
    project_brief_id uuid,
    test_command text DEFAULT ''::text NOT NULL,
    CONSTRAINT tickets_priority_check CHECK ((priority = ANY (ARRAY['critical'::text, 'high'::text, 'normal'::text, 'low'::text]))),
    CONSTRAINT tickets_status_check CHECK ((status = ANY (ARRAY['todo'::text, 'in_progress'::text, 'blocked'::text, 'done'::text])))
);

--
-- Name: tool_approvals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.tool_approvals (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id text NOT NULL,
    agent_id text NOT NULL,
    tool_name text NOT NULL,
    tool_args jsonb DEFAULT '{}'::jsonb NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    decided_at timestamp with time zone,
    CONSTRAINT tool_approvals_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'expired'::text])))
);

--
-- Name: traces; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.traces (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    session_key text,
    start_time timestamp with time zone DEFAULT now() NOT NULL,
    end_time timestamp with time zone,
    duration_ms integer,
    total_input_tokens integer DEFAULT 0,
    total_output_tokens integer DEFAULT 0,
    total_cost_cents integer DEFAULT 0,
    status character varying(20) DEFAULT 'running'::character varying,
    error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.traces FORCE ROW LEVEL SECURITY;

--
-- Name: user_presence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.user_presence (
    user_id uuid NOT NULL,
    tenant_id uuid NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    is_online boolean DEFAULT false NOT NULL,
    channel text DEFAULT 'web'::text NOT NULL
);

--
-- Name: user_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.user_sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    ip_address text,
    user_agent text,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid DEFAULT '00000000-0000-0000-0000-000000000001'::uuid NOT NULL,
    username character varying(100) NOT NULL,
    email character varying(255),
    password_hash text NOT NULL,
    role character varying(20) DEFAULT 'admin'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    failed_logins integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    timezone text DEFAULT 'UTC'::text NOT NULL
);

--
-- Name: voice_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.voice_providers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    driver text NOT NULL,
    api_base text DEFAULT ''::text NOT NULL,
    api_key text DEFAULT ''::text NOT NULL,
    settings jsonb DEFAULT '{}'::jsonb NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT voice_providers_kind_check CHECK ((kind = ANY (ARRAY['tts'::text, 'stt'::text, 'realtime'::text])))
);

--
-- Name: wakeup_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.wakeup_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    agent_id text NOT NULL,
    source text NOT NULL,
    actor_type text NOT NULL,
    actor_id text,
    reason text,
    context jsonb,
    priority integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    error_msg text,
    cause text,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    session_id uuid,
    plan_id uuid,
    node_id uuid,
    consumed_at timestamp with time zone,
    attempts integer DEFAULT 0 NOT NULL,
    dead_letter_reason text,
    CONSTRAINT wakeup_cause_chk CHECK (((cause IS NULL) OR (cause = ANY (ARRAY['issue_assigned'::text, 'mention_routed'::text, 'cron_fired'::text, 'approval_resolved'::text, 'plan_node_ready'::text, 'manual'::text]))))
);

ALTER TABLE ONLY public.wakeup_requests FORCE ROW LEVEL SECURITY;

--
-- Name: wasm_plugins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.wasm_plugins (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    wasm_binary bytea NOT NULL,
    sha256 text NOT NULL,
    parameters jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    CONSTRAINT wasm_plugins_name_shape_chk CHECK ((name ~ '^[a-z][a-z0-9_]{0,62}$'::text)),
    CONSTRAINT wasm_plugins_sha256_shape_chk CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text))
);

ALTER TABLE ONLY public.wasm_plugins FORCE ROW LEVEL SECURITY;

--
-- Name: TABLE wasm_plugins; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.wasm_plugins IS 'Tenant-scoped Wasm plugin registry. Admins upload via POST /v1/plugins. Orchestrator loads per-tenant plugins at plan execution time via plugins.Loader.';

--
-- Name: COLUMN wasm_plugins.sha256; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.wasm_plugins.sha256 IS 'SHA256 of wasm_binary, computed at upload. Loader verifies before compile to catch tampering or column-level corruption.';

--
-- Name: COLUMN wasm_plugins.revoked_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.wasm_plugins.revoked_at IS 'Soft-delete marker. Active plugins have NULL here. Revoking does NOT DROP the row — operators retain it for audit.';

--
-- Name: whatsapp_pending_senders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.whatsapp_pending_senders (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    channel_id uuid NOT NULL,
    sender_jid text NOT NULL,
    display_name text DEFAULT ''::text NOT NULL,
    otp_code text NOT NULL,
    otp_attempts integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    original_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: work_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.work_categories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    slug text NOT NULL,
    description text,
    icon text,
    color text DEFAULT 'violet'::text,
    display_order integer DEFAULT 0
);

--
-- Name: work_goals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.work_goals (
    id uuid DEFAULT public.uuid_generate_v7() NOT NULL,
    tenant_id uuid NOT NULL,
    title text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    parent_id uuid,
    order_index integer DEFAULT 0 NOT NULL,
    status text DEFAULT 'open'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT work_goals_status_check CHECK ((status = ANY (ARRAY['open'::text, 'done'::text])))
);

--
-- Name: workflow_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.workflow_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    workflow_id uuid,
    tenant_id uuid NOT NULL,
    agent_id uuid,
    status text DEFAULT 'running'::text,
    current_step integer DEFAULT 0,
    context jsonb DEFAULT '{}'::jsonb,
    result text DEFAULT ''::text,
    started_at timestamp with time zone DEFAULT now(),
    completed_at timestamp with time zone,
    error text
);

--
-- Name: workflows; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.workflows (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id uuid NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text,
    agent_id uuid,
    trigger_type text DEFAULT 'manual'::text NOT NULL,
    trigger_config jsonb DEFAULT '{}'::jsonb,
    steps jsonb DEFAULT '[]'::jsonb NOT NULL,
    variables jsonb DEFAULT '{}'::jsonb,
    enabled boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: workspace_dashboards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE IF NOT EXISTS public.workspace_dashboards (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    template_id text NOT NULL,
    name text NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);

--
-- Name: audit_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_log ALTER COLUMN id SET DEFAULT nextval('public.audit_log_id_seq'::regclass);

--
-- Name: cost_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_events ALTER COLUMN id SET DEFAULT nextval('public.cost_events_id_seq'::regclass);

--
-- Name: supervisor_fix_history id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_fix_history ALTER COLUMN id SET DEFAULT nextval('public.supervisor_fix_history_id_seq'::regclass);

--
-- Name: agent_bundles agent_bundles_agent_id_bundle_type_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_bundles
    ADD CONSTRAINT agent_bundles_agent_id_bundle_type_name_key UNIQUE (agent_id, bundle_type, name);

--
-- Name: agent_bundles agent_bundles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_bundles
    ADD CONSTRAINT agent_bundles_pkey PRIMARY KEY (id);

--
-- Name: agent_channel_bindings agent_channel_bindings_instance_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_channel_bindings
    ADD CONSTRAINT agent_channel_bindings_instance_id_key UNIQUE (instance_id);

--
-- Name: agent_channel_bindings agent_channel_bindings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_channel_bindings
    ADD CONSTRAINT agent_channel_bindings_pkey PRIMARY KEY (id);

--
-- Name: agent_messages agent_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_pkey PRIMARY KEY (id);

--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

--
-- Name: agents agents_tenant_id_agent_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_tenant_id_agent_key_key UNIQUE (tenant_id, agent_key);

--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);

--
-- Name: app_schema_migrations app_schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_schema_migrations
    ADD CONSTRAINT app_schema_migrations_pkey PRIMARY KEY (app_slug, tenant_id, version);

--
-- Name: approval_comments approval_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_comments
    ADD CONSTRAINT approval_comments_pkey PRIMARY KEY (id);

--
-- Name: approvals approvals_node_id_state_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_node_id_state_key UNIQUE (node_id, state);

--
-- Name: approvals approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_pkey PRIMARY KEY (id);

--
-- Name: apps apps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_pkey PRIMARY KEY (id);

--
-- Name: apps apps_tenant_slug_uniq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.apps
    ADD CONSTRAINT apps_tenant_slug_uniq UNIQUE (tenant_id, slug);

--
-- Name: audit_log audit_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_log
    ADD CONSTRAINT audit_log_pkey PRIMARY KEY (id);

--
-- Name: brief_agent_spend brief_agent_spend_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brief_agent_spend
    ADD CONSTRAINT brief_agent_spend_pkey PRIMARY KEY (id);

--
-- Name: builtin_tools builtin_tools_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.builtin_tools
    ADD CONSTRAINT builtin_tools_pkey PRIMARY KEY (name);

--
-- Name: calendar_events calendar_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_pkey PRIMARY KEY (id);

--
-- Name: category_model_assignments category_model_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.category_model_assignments
    ADD CONSTRAINT category_model_assignments_pkey PRIMARY KEY (tenant_id, category_slug, model_id);

--
-- Name: channel_instances channel_instances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_instances
    ADD CONSTRAINT channel_instances_pkey PRIMARY KEY (id);

--
-- Name: channel_instances channel_instances_tenant_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_instances
    ADD CONSTRAINT channel_instances_tenant_id_name_key UNIQUE (tenant_id, name);

--
-- Name: config_secrets config_secrets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_secrets
    ADD CONSTRAINT config_secrets_pkey PRIMARY KEY (tenant_id, key);

--
-- Name: connector_actions connector_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_actions
    ADD CONSTRAINT connector_actions_pkey PRIMARY KEY (id);

--
-- Name: connector_actions connector_actions_platform_id_action_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_actions
    ADD CONSTRAINT connector_actions_platform_id_action_key_key UNIQUE (platform_id, action_key);

--
-- Name: connector_credentials connector_credentials_agent_id_connector_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_credentials
    ADD CONSTRAINT connector_credentials_agent_id_connector_id_key UNIQUE (agent_id, connector_id);

--
-- Name: connector_credentials connector_credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_credentials
    ADD CONSTRAINT connector_credentials_pkey PRIMARY KEY (id);

--
-- Name: connector_platforms connector_platforms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_platforms
    ADD CONSTRAINT connector_platforms_pkey PRIMARY KEY (id);

--
-- Name: connector_snapshots connector_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_snapshots
    ADD CONSTRAINT connector_snapshots_pkey PRIMARY KEY (id);

--
-- Name: contacts contacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contacts
    ADD CONSTRAINT contacts_pkey PRIMARY KEY (id);

--
-- Name: contacts contacts_tenant_id_external_id_channel_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contacts
    ADD CONSTRAINT contacts_tenant_id_external_id_channel_key UNIQUE (tenant_id, external_id, channel);

--
-- Name: cost_events cost_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cost_events
    ADD CONSTRAINT cost_events_pkey PRIMARY KEY (id);

--
-- Name: credentials credentials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_pkey PRIMARY KEY (id);

--
-- Name: credentials credentials_tenant_id_platform_id_label_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_tenant_id_platform_id_label_key UNIQUE (tenant_id, platform_id, label);

--
-- Name: crew_members crew_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crew_members
    ADD CONSTRAINT crew_members_pkey PRIMARY KEY (crew_id, agent_id);

--
-- Name: crews crews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crews
    ADD CONSTRAINT crews_pkey PRIMARY KEY (id);

--
-- Name: cron_jobs cron_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cron_jobs
    ADD CONSTRAINT cron_jobs_pkey PRIMARY KEY (id);

--
-- Name: crystallized_skills crystallized_skills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crystallized_skills
    ADD CONSTRAINT crystallized_skills_pkey PRIMARY KEY (id);

--
-- Name: custom_tools custom_tools_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_tools
    ADD CONSTRAINT custom_tools_pkey PRIMARY KEY (id);

--
-- Name: custom_tools custom_tools_tenant_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_tools
    ADD CONSTRAINT custom_tools_tenant_id_name_key UNIQUE (tenant_id, name);

--
-- Name: daemon_agents daemon_agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_agents
    ADD CONSTRAINT daemon_agents_pkey PRIMARY KEY (id);

--
-- Name: daemon_plans daemon_plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_plans
    ADD CONSTRAINT daemon_plans_pkey PRIMARY KEY (id);

--
-- Name: daemon_tasks daemon_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_tasks
    ADD CONSTRAINT daemon_tasks_pkey PRIMARY KEY (id);

--
-- Name: deployment_config deployment_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deployment_config
    ADD CONSTRAINT deployment_config_pkey PRIMARY KEY (key);

--
-- Name: discussions discussions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_pkey PRIMARY KEY (id);

--
-- Name: document_chunks document_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.document_chunks
    ADD CONSTRAINT document_chunks_pkey PRIMARY KEY (id);

--
-- Name: draft_replies draft_replies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.draft_replies
    ADD CONSTRAINT draft_replies_pkey PRIMARY KEY (id);

--
-- Name: drive_files drive_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.drive_files
    ADD CONSTRAINT drive_files_pkey PRIMARY KEY (id);

--
-- Name: drive_permissions drive_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.drive_permissions
    ADD CONSTRAINT drive_permissions_pkey PRIMARY KEY (id);

--
-- Name: email_routing email_routing_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_routing
    ADD CONSTRAINT email_routing_pkey PRIMARY KEY (id);

--
-- Name: email_routing email_routing_tenant_id_shared_mailbox_alias_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_routing
    ADD CONSTRAINT email_routing_tenant_id_shared_mailbox_alias_key UNIQUE (tenant_id, shared_mailbox, alias);

--
-- Name: evaluations evaluations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.evaluations
    ADD CONSTRAINT evaluations_pkey PRIMARY KEY (id);

--
-- Name: feedback feedback_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feedback
    ADD CONSTRAINT feedback_pkey PRIMARY KEY (id);

--
-- Name: github_connections github_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_connections
    ADD CONSTRAINT github_connections_pkey PRIMARY KEY (id);

--
-- Name: github_connections github_connections_tenant_id_agent_id_owner_repo_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_connections
    ADD CONSTRAINT github_connections_tenant_id_agent_id_owner_repo_key UNIQUE (tenant_id, agent_id, owner, repo);

--
-- Name: github_events github_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_events
    ADD CONSTRAINT github_events_pkey PRIMARY KEY (id);

--
-- Name: goals goals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals
    ADD CONSTRAINT goals_pkey PRIMARY KEY (id);

--
-- Name: heartbeat_configs heartbeat_configs_agent_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_configs
    ADD CONSTRAINT heartbeat_configs_agent_id_key UNIQUE (agent_id);

--
-- Name: heartbeat_configs heartbeat_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_configs
    ADD CONSTRAINT heartbeat_configs_pkey PRIMARY KEY (id);

--
-- Name: heartbeat_queue heartbeat_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_queue
    ADD CONSTRAINT heartbeat_queue_pkey PRIMARY KEY (id);

--
-- Name: heartbeat_runs heartbeat_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_runs
    ADD CONSTRAINT heartbeat_runs_pkey PRIMARY KEY (id);

--
-- Name: inbound_agent_config inbound_agent_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_agent_config
    ADD CONSTRAINT inbound_agent_config_pkey PRIMARY KEY (agent_id);

--
-- Name: inbound_rules inbound_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_rules
    ADD CONSTRAINT inbound_rules_pkey PRIMARY KEY (id);

--
-- Name: key_usage_log key_usage_log_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_usage_log
    ADD CONSTRAINT key_usage_log_pkey PRIMARY KEY (id);

--
-- Name: kg_entities kg_entities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_entities
    ADD CONSTRAINT kg_entities_pkey PRIMARY KEY (id);

--
-- Name: kg_relationships kg_relationships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_relationships
    ADD CONSTRAINT kg_relationships_pkey PRIMARY KEY (id);

--
-- Name: learned_skills learned_skills_agent_id_pattern_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learned_skills
    ADD CONSTRAINT learned_skills_agent_id_pattern_hash_key UNIQUE (agent_id, pattern_hash);

--
-- Name: learned_skills learned_skills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learned_skills
    ADD CONSTRAINT learned_skills_pkey PRIMARY KEY (id);

--
-- Name: llm_stats_cache llm_stats_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.llm_stats_cache
    ADD CONSTRAINT llm_stats_cache_pkey PRIMARY KEY (key);

--
-- Name: magic_links magic_links_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_links
    ADD CONSTRAINT magic_links_pkey PRIMARY KEY (id);

--
-- Name: magic_links magic_links_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_links
    ADD CONSTRAINT magic_links_token_key UNIQUE (token);

--
-- Name: mail_aliases mail_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_aliases
    ADD CONSTRAINT mail_aliases_pkey PRIMARY KEY (id);

--
-- Name: mail_approval_queue mail_approval_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_approval_queue
    ADD CONSTRAINT mail_approval_queue_pkey PRIMARY KEY (id);

--
-- Name: mail_routing_rules mail_routing_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_routing_rules
    ADD CONSTRAINT mail_routing_rules_pkey PRIMARY KEY (id);

--
-- Name: mail_thread_assignments mail_thread_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_thread_assignments
    ADD CONSTRAINT mail_thread_assignments_pkey PRIMARY KEY (id);

--
-- Name: mailbox_messages mailbox_messages_message_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_messages
    ADD CONSTRAINT mailbox_messages_message_id_key UNIQUE (message_id);

--
-- Name: mailbox_messages mailbox_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_messages
    ADD CONSTRAINT mailbox_messages_pkey PRIMARY KEY (id);

--
-- Name: media_providers media_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.media_providers
    ADD CONSTRAINT media_providers_pkey PRIMARY KEY (id);

--
-- Name: memories memories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_pkey PRIMARY KEY (id);

--
-- Name: memory_backend_config memory_backend_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_backend_config
    ADD CONSTRAINT memory_backend_config_pkey PRIMARY KEY (tenant_id);

--
-- Name: memory_bulletins memory_bulletins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_bulletins
    ADD CONSTRAINT memory_bulletins_pkey PRIMARY KEY (id);

--
-- Name: memory_chunks memory_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_chunks
    ADD CONSTRAINT memory_chunks_pkey PRIMARY KEY (id);

--
-- Name: memory_documents memory_documents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_documents
    ADD CONSTRAINT memory_documents_pkey PRIMARY KEY (id);

--
-- Name: memory_edges memory_edges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_edges
    ADD CONSTRAINT memory_edges_pkey PRIMARY KEY (id);

--
-- Name: memory_edges memory_edges_source_id_target_id_edge_type_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_edges
    ADD CONSTRAINT memory_edges_source_id_target_id_edge_type_key UNIQUE (source_id, target_id, edge_type);

--
-- Name: model_discoveries model_discoveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_discoveries
    ADD CONSTRAINT model_discoveries_pkey PRIMARY KEY (id);

--
-- Name: model_discoveries model_discoveries_tenant_id_provider_id_model_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_discoveries
    ADD CONSTRAINT model_discoveries_tenant_id_provider_id_model_id_key UNIQUE (tenant_id, provider_id, model_id);

--
-- Name: model_pricing model_pricing_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_pricing
    ADD CONSTRAINT model_pricing_pkey PRIMARY KEY (model_id);

--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

--
-- Name: outbound_queue outbound_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.outbound_queue
    ADD CONSTRAINT outbound_queue_pkey PRIMARY KEY (id);

--
-- Name: paired_devices paired_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.paired_devices
    ADD CONSTRAINT paired_devices_pkey PRIMARY KEY (id);

--
-- Name: paired_devices paired_devices_tenant_id_sender_id_channel_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.paired_devices
    ADD CONSTRAINT paired_devices_tenant_id_sender_id_channel_key UNIQUE (tenant_id, sender_id, channel);

--
-- Name: pairing_requests pairing_requests_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pairing_requests
    ADD CONSTRAINT pairing_requests_code_key UNIQUE (code);

--
-- Name: pairing_requests pairing_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pairing_requests
    ADD CONSTRAINT pairing_requests_pkey PRIMARY KEY (id);

--
-- Name: permission_requests permission_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_requests
    ADD CONSTRAINT permission_requests_pkey PRIMARY KEY (id);

--
-- Name: pinned_tiles pinned_tiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pinned_tiles
    ADD CONSTRAINT pinned_tiles_pkey PRIMARY KEY (id);

--
-- Name: plan_edges plan_edges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_edges
    ADD CONSTRAINT plan_edges_pkey PRIMARY KEY (plan_id, from_node, to_node, condition);

--
-- Name: plan_nodes plan_nodes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_nodes
    ADD CONSTRAINT plan_nodes_pkey PRIMARY KEY (id);

--
-- Name: plans plans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plans
    ADD CONSTRAINT plans_pkey PRIMARY KEY (id);

--
-- Name: prime_delegations prime_delegations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prime_delegations
    ADD CONSTRAINT prime_delegations_pkey PRIMARY KEY (id);

--
-- Name: project_briefs project_briefs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_briefs
    ADD CONSTRAINT project_briefs_pkey PRIMARY KEY (id);

--
-- Name: prompt_cache_stats prompt_cache_stats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_cache_stats
    ADD CONSTRAINT prompt_cache_stats_pkey PRIMARY KEY (agent_id, session_id);

--
-- Name: provider_keys provider_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provider_keys
    ADD CONSTRAINT provider_keys_pkey PRIMARY KEY (id);

--
-- Name: provider_pool_config provider_pool_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provider_pool_config
    ADD CONSTRAINT provider_pool_config_pkey PRIMARY KEY (tenant_id, provider_id);

--
-- Name: providers providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.providers
    ADD CONSTRAINT providers_pkey PRIMARY KEY (id);

--
-- Name: providers providers_tenant_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.providers
    ADD CONSTRAINT providers_tenant_id_name_key UNIQUE (tenant_id, name);

--
-- Name: qoros_daily_logs qoros_daily_logs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.qoros_daily_logs
    ADD CONSTRAINT qoros_daily_logs_pkey PRIMARY KEY (id);

--
-- Name: qoros_state qoros_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.qoros_state
    ADD CONSTRAINT qoros_state_pkey PRIMARY KEY (agent_id);

--
-- Name: refresh_tokens refresh_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);

--
-- Name: refresh_tokens refresh_tokens_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_key UNIQUE (token);

--
-- Name: room_decisions room_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_decisions
    ADD CONSTRAINT room_decisions_pkey PRIMARY KEY (id);

--
-- Name: room_members room_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_members
    ADD CONSTRAINT room_members_pkey PRIMARY KEY (room_id, agent_id);

--
-- Name: room_messages room_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_messages
    ADD CONSTRAINT room_messages_pkey PRIMARY KEY (id);

--
-- Name: room_minutes room_minutes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_minutes
    ADD CONSTRAINT room_minutes_pkey PRIMARY KEY (id);

--
-- Name: room_tasks room_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_tasks
    ADD CONSTRAINT room_tasks_pkey PRIMARY KEY (id);

--
-- Name: room_typing room_typing_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_typing
    ADD CONSTRAINT room_typing_pkey PRIMARY KEY (room_id, agent_id);

--
-- Name: rooms rooms_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.rooms
    ADD CONSTRAINT rooms_pkey PRIMARY KEY (id);

--
-- Name: routing_decisions routing_decisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.routing_decisions
    ADD CONSTRAINT routing_decisions_pkey PRIMARY KEY (id);

--
-- Name: running_apps running_apps_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.running_apps
    ADD CONSTRAINT running_apps_pkey PRIMARY KEY (id);

--
-- Name: running_apps running_apps_proxy_prefix_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.running_apps
    ADD CONSTRAINT running_apps_proxy_prefix_key UNIQUE (proxy_prefix);

--
-- Name: sandbox_runs sandbox_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_runs
    ADD CONSTRAINT sandbox_runs_pkey PRIMARY KEY (id);

--
-- Name: scenarios scenarios_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.scenarios
    ADD CONSTRAINT scenarios_pkey PRIMARY KEY (id);

--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.schema_migrations
    DROP CONSTRAINT IF EXISTS schema_migrations_pkey;
ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);

--
-- Name: schema_version schema_version_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_version
    ADD CONSTRAINT schema_version_pkey PRIMARY KEY (version);

--
-- Name: selected_models selected_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.selected_models
    ADD CONSTRAINT selected_models_pkey PRIMARY KEY (id);

--
-- Name: selected_models selected_models_tenant_id_provider_id_model_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.selected_models
    ADD CONSTRAINT selected_models_tenant_id_provider_id_model_id_key UNIQUE (tenant_id, provider_id, model_id);

--
-- Name: service_accounts service_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_accounts
    ADD CONSTRAINT service_accounts_pkey PRIMARY KEY (id);

--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);

--
-- Name: sessions sessions_tenant_id_session_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_tenant_id_session_key_key UNIQUE (tenant_id, session_key);

--
-- Name: skill_installations skill_installations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_installations
    ADD CONSTRAINT skill_installations_pkey PRIMARY KEY (id);

--
-- Name: skill_installations skill_installations_tenant_id_manifest_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_installations
    ADD CONSTRAINT skill_installations_tenant_id_manifest_id_key UNIQUE (tenant_id, manifest_id);

--
-- Name: skill_manifests skill_manifests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_manifests
    ADD CONSTRAINT skill_manifests_pkey PRIMARY KEY (id);

--
-- Name: skill_reviews skill_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_reviews
    ADD CONSTRAINT skill_reviews_pkey PRIMARY KEY (id);

--
-- Name: skills skills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT skills_pkey PRIMARY KEY (id);

--
-- Name: skills skills_tenant_id_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT skills_tenant_id_slug_key UNIQUE (tenant_id, slug);

--
-- Name: social_autoposts social_autoposts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_autoposts
    ADD CONSTRAINT social_autoposts_pkey PRIMARY KEY (id);

--
-- Name: social_integrations social_integrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_integrations
    ADD CONSTRAINT social_integrations_pkey PRIMARY KEY (id);

--
-- Name: social_posts social_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_posts
    ADD CONSTRAINT social_posts_pkey PRIMARY KEY (id);

--
-- Name: soul_mail_identities soul_mail_identities_address_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_mail_identities
    ADD CONSTRAINT soul_mail_identities_address_key UNIQUE (address);

--
-- Name: soul_mail_identities soul_mail_identities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_mail_identities
    ADD CONSTRAINT soul_mail_identities_pkey PRIMARY KEY (id);

--
-- Name: soul_skills soul_skills_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_skills
    ADD CONSTRAINT soul_skills_pkey PRIMARY KEY (agent_id, skill_id);

--
-- Name: soul_usage soul_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_usage
    ADD CONSTRAINT soul_usage_pkey PRIMARY KEY (id);

--
-- Name: spans spans_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spans
    ADD CONSTRAINT spans_pkey PRIMARY KEY (id);

--
-- Name: stream_checkpoints stream_checkpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.stream_checkpoints
    ADD CONSTRAINT stream_checkpoints_pkey PRIMARY KEY (session_id);

--
-- Name: subagent_runs subagent_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subagent_runs
    ADD CONSTRAINT subagent_runs_pkey PRIMARY KEY (id);

--
-- Name: supervisor_exchanges supervisor_exchanges_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_exchanges
    ADD CONSTRAINT supervisor_exchanges_pkey PRIMARY KEY (id);

--
-- Name: supervisor_fix_history supervisor_fix_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_fix_history
    ADD CONSTRAINT supervisor_fix_history_pkey PRIMARY KEY (id);

--
-- Name: supervisor_messages supervisor_messages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_messages
    ADD CONSTRAINT supervisor_messages_pkey PRIMARY KEY (id);

--
-- Name: supervisor_reviews supervisor_reviews_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_reviews
    ADD CONSTRAINT supervisor_reviews_pkey PRIMARY KEY (id);

--
-- Name: system_configs system_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_configs
    ADD CONSTRAINT system_configs_pkey PRIMARY KEY (id);

--
-- Name: system_configs system_configs_tenant_id_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_configs
    ADD CONSTRAINT system_configs_tenant_id_key_key UNIQUE (tenant_id, key);

--
-- Name: task_comments task_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_pkey PRIMARY KEY (id);

--
-- Name: task_events task_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_events
    ADD CONSTRAINT task_events_pkey PRIMARY KEY (id);

--
-- Name: task_files task_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_files
    ADD CONSTRAINT task_files_pkey PRIMARY KEY (id);

--
-- Name: tasks tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

--
-- Name: team_task_attachments team_task_attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_attachments
    ADD CONSTRAINT team_task_attachments_pkey PRIMARY KEY (id);

--
-- Name: team_task_comments team_task_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_comments
    ADD CONSTRAINT team_task_comments_pkey PRIMARY KEY (id);

--
-- Name: team_task_events team_task_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_events
    ADD CONSTRAINT team_task_events_pkey PRIMARY KEY (id);

--
-- Name: tenant_skill_config tenant_skill_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tenant_skill_config
    ADD CONSTRAINT tenant_skill_config_pkey PRIMARY KEY (tenant_id, skill_slug);

--
-- Name: tenants tenants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tenants
    ADD CONSTRAINT tenants_pkey PRIMARY KEY (id);

--
-- Name: tenants tenants_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tenants
    ADD CONSTRAINT tenants_slug_key UNIQUE (slug);

--
-- Name: ticket_comments ticket_comments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_comments
    ADD CONSTRAINT ticket_comments_pkey PRIMARY KEY (id);

--
-- Name: ticket_counters ticket_counters_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_counters
    ADD CONSTRAINT ticket_counters_pkey PRIMARY KEY (tenant_id);

--
-- Name: ticket_files ticket_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_files
    ADD CONSTRAINT ticket_files_pkey PRIMARY KEY (id);

--
-- Name: tickets tickets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_pkey PRIMARY KEY (id);

--
-- Name: tool_approvals tool_approvals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tool_approvals
    ADD CONSTRAINT tool_approvals_pkey PRIMARY KEY (id);

--
-- Name: traces traces_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.traces
    ADD CONSTRAINT traces_pkey PRIMARY KEY (id);

--
-- Name: user_presence user_presence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_presence
    ADD CONSTRAINT user_presence_pkey PRIMARY KEY (user_id);

--
-- Name: user_sessions user_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);

--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);

--
-- Name: voice_providers voice_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voice_providers
    ADD CONSTRAINT voice_providers_pkey PRIMARY KEY (id);

--
-- Name: voice_providers voice_providers_tenant_id_name_kind_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voice_providers
    ADD CONSTRAINT voice_providers_tenant_id_name_kind_key UNIQUE (tenant_id, name, kind);

--
-- Name: wakeup_requests wakeup_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wakeup_requests
    ADD CONSTRAINT wakeup_requests_pkey PRIMARY KEY (id);

--
-- Name: wasm_plugins wasm_plugins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wasm_plugins
    ADD CONSTRAINT wasm_plugins_pkey PRIMARY KEY (id);

--
-- Name: whatsapp_pending_senders whatsapp_pending_senders_channel_id_sender_jid_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_pending_senders
    ADD CONSTRAINT whatsapp_pending_senders_channel_id_sender_jid_key UNIQUE (channel_id, sender_jid);

--
-- Name: whatsapp_pending_senders whatsapp_pending_senders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_pending_senders
    ADD CONSTRAINT whatsapp_pending_senders_pkey PRIMARY KEY (id);

--
-- Name: work_categories work_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_categories
    ADD CONSTRAINT work_categories_pkey PRIMARY KEY (id);

--
-- Name: work_categories work_categories_tenant_id_slug_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_categories
    ADD CONSTRAINT work_categories_tenant_id_slug_key UNIQUE (tenant_id, slug);

--
-- Name: work_goals work_goals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_goals
    ADD CONSTRAINT work_goals_pkey PRIMARY KEY (id);

--
-- Name: workflow_runs workflow_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_runs
    ADD CONSTRAINT workflow_runs_pkey PRIMARY KEY (id);

--
-- Name: workflows workflows_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflows
    ADD CONSTRAINT workflows_pkey PRIMARY KEY (id);

--
-- Name: workspace_dashboards workspace_dashboards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspace_dashboards
    ADD CONSTRAINT workspace_dashboards_pkey PRIMARY KEY (id);

--
-- Name: workspace_dashboards workspace_dashboards_tenant_id_template_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workspace_dashboards
    ADD CONSTRAINT workspace_dashboards_tenant_id_template_id_key UNIQUE (tenant_id, template_id);

--
-- Name: heartbeat_queue_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS heartbeat_queue_pending ON public.heartbeat_queue USING btree (status, run_at) WHERE (status = 'pending'::text);

--
-- Name: idx_acb_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_acb_agent ON public.agent_channel_bindings USING btree (agent_id);

--
-- Name: idx_acb_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_acb_tenant ON public.agent_channel_bindings USING btree (tenant_id);

--
-- Name: idx_agent_msgs_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_agent_msgs_task ON public.agent_messages USING btree (task_id);

--
-- Name: idx_agent_msgs_to; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_agent_msgs_to ON public.agent_messages USING btree (to_agent, read);

--
-- Name: idx_agents_brief; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_agents_brief ON public.agents USING btree (project_brief_id) WHERE (project_brief_id IS NOT NULL);

--
-- Name: idx_agents_manager; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_agents_manager ON public.agents USING btree (manager_id) WHERE (manager_id IS NOT NULL);

--
-- Name: idx_agents_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_agents_tenant ON public.agents USING btree (tenant_id) WHERE (deleted_at IS NULL);

--
-- Name: idx_approval_comments_approval; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_approval_comments_approval ON public.approval_comments USING btree (approval_id, created_at);

--
-- Name: idx_approvals_plan_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_approvals_plan_state ON public.approvals USING btree (plan_id, state);

--
-- Name: idx_apps_scope_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_apps_scope_agent ON public.apps USING btree (tenant_id, scope, owner_agent_id) WHERE (scope = 'agent'::text);

--
-- Name: idx_apps_scope_team; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_apps_scope_team ON public.apps USING btree (tenant_id, scope, owner_team_id) WHERE (scope = 'team'::text);

--
-- Name: idx_audit_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_actor ON public.audit_log USING btree (actor_id, created_at DESC);

--
-- Name: idx_audit_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_resource ON public.audit_log USING btree (resource, resource_id);

--
-- Name: idx_audit_tenant_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_audit_tenant_time ON public.audit_log USING btree (tenant_id, created_at DESC);

--
-- Name: idx_brief_spend_brief; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_brief_spend_brief ON public.brief_agent_spend USING btree (brief_id);

--
-- Name: idx_builtin_tools_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_builtin_tools_category ON public.builtin_tools USING btree (category);

--
-- Name: idx_builtin_tools_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_builtin_tools_enabled ON public.builtin_tools USING btree (enabled);

--
-- Name: idx_bulletins_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bulletins_agent ON public.memory_bulletins USING btree (agent_id, generated_at DESC);

--
-- Name: idx_bundles_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_bundles_agent ON public.agent_bundles USING btree (agent_id);

--
-- Name: idx_calendar_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_calendar_agent ON public.calendar_events USING btree (agent_id);

--
-- Name: idx_calendar_range; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_calendar_range ON public.calendar_events USING btree (start_at, end_at);

--
-- Name: idx_calendar_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_calendar_tenant ON public.calendar_events USING btree (tenant_id);

--
-- Name: idx_calendar_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_calendar_type ON public.calendar_events USING btree (event_type);

--
-- Name: idx_channel_instances_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_channel_instances_agent ON public.channel_instances USING btree (agent_id);

--
-- Name: idx_channel_instances_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_channel_instances_type ON public.channel_instances USING btree (channel_type);

--
-- Name: idx_channels_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_channels_tenant ON public.channel_instances USING btree (tenant_id);

--
-- Name: idx_chunks_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chunks_agent ON public.document_chunks USING btree (agent_id);

--
-- Name: idx_chunks_embedding; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chunks_embedding ON public.document_chunks USING ivfflat (embedding public.vector_cosine_ops) WITH (lists='100');

--
-- Name: idx_chunks_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_chunks_source ON public.document_chunks USING btree (source_type, source_id);

--
-- Name: idx_conn_creds_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_conn_creds_agent ON public.connector_credentials USING btree (agent_id);

--
-- Name: idx_conn_creds_connector; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_conn_creds_connector ON public.connector_credentials USING btree (connector_id);

--
-- Name: idx_connector_actions_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_connector_actions_platform ON public.connector_actions USING btree (platform_id);

--
-- Name: idx_connector_snapshots_lookup; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_connector_snapshots_lookup ON public.connector_snapshots USING btree (tenant_id, source_slug, created_at DESC);

--
-- Name: idx_contacts_tenant_last_seen; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_contacts_tenant_last_seen ON public.contacts USING btree (tenant_id, last_seen DESC);

--
-- Name: idx_contacts_tenant_stage; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_contacts_tenant_stage ON public.contacts USING btree (tenant_id, pipeline_stage);

--
-- Name: idx_cost_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cost_agent ON public.cost_events USING btree (agent_id, created_at DESC);

--
-- Name: idx_cost_model; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cost_model ON public.cost_events USING btree (model);

--
-- Name: idx_cost_tenant_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cost_tenant_time ON public.cost_events USING btree (tenant_id, created_at DESC);

--
-- Name: idx_cost_trace; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cost_trace ON public.cost_events USING btree (trace_id) WHERE (trace_id <> ''::text);

--
-- Name: idx_credentials_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_credentials_platform ON public.credentials USING btree (tenant_id, platform_id);

--
-- Name: idx_credentials_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_credentials_tenant ON public.credentials USING btree (tenant_id);

--
-- Name: idx_crew_members_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_crew_members_agent ON public.crew_members USING btree (agent_id);

--
-- Name: idx_crews_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_crews_tenant ON public.crews USING btree (tenant_id);

--
-- Name: idx_cron_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cron_tenant ON public.cron_jobs USING btree (tenant_id);

--
-- Name: idx_cryst_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cryst_agent ON public.crystallized_skills USING btree (agent_id);

--
-- Name: idx_cryst_reuse; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cryst_reuse ON public.crystallized_skills USING btree (reuse_count DESC);

--
-- Name: idx_cryst_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_cryst_scope ON public.crystallized_skills USING btree (scope);

--
-- Name: idx_custom_tools_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_custom_tools_tenant ON public.custom_tools USING btree (tenant_id);

--
-- Name: idx_daemon_agents_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_agents_provider ON public.daemon_agents USING btree (tenant_id, provider);

--
-- Name: idx_daemon_agents_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_agents_tenant ON public.daemon_agents USING btree (tenant_id, status);

--
-- Name: idx_daemon_plans_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_plans_tenant ON public.daemon_plans USING btree (tenant_id, status, created_at DESC);

--
-- Name: idx_daemon_tasks_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_tasks_owner ON public.daemon_tasks USING btree (owner, status);

--
-- Name: idx_daemon_tasks_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_tasks_plan ON public.daemon_tasks USING btree (plan_id);

--
-- Name: idx_daemon_tasks_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daemon_tasks_tenant ON public.daemon_tasks USING btree (tenant_id, status, created_at DESC);

--
-- Name: idx_daily_logs_agent_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daily_logs_agent_date ON public.qoros_daily_logs USING btree (agent_id, log_date DESC);

--
-- Name: idx_daily_logs_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_daily_logs_tenant ON public.qoros_daily_logs USING btree (tenant_id, log_date DESC);

--
-- Name: idx_discoveries_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_discoveries_tenant ON public.model_discoveries USING btree (tenant_id, user_action, first_seen_at DESC);

--
-- Name: idx_discussions_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_discussions_agent ON public.discussions USING btree (agent_id, last_active_at DESC);

--
-- Name: idx_discussions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_discussions_tenant ON public.discussions USING btree (tenant_id);

--
-- Name: idx_draft_replies_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_draft_replies_agent ON public.draft_replies USING btree (agent_id, status, created_at DESC);

--
-- Name: idx_drive_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_drive_agent ON public.drive_files USING btree (agent_id, parent_id);

--
-- Name: idx_drive_enrichment_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_drive_enrichment_status ON public.drive_files USING btree (enrichment_status);

--
-- Name: idx_evaluations_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_evaluations_agent ON public.evaluations USING btree (agent_id);

--
-- Name: idx_evaluations_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_evaluations_session ON public.evaluations USING btree (session_id);

--
-- Name: idx_feedback_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_feedback_agent ON public.feedback USING btree (agent_id);

--
-- Name: idx_feedback_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_feedback_session ON public.feedback USING btree (session_id);

--
-- Name: idx_feedback_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_feedback_tenant ON public.feedback USING btree (tenant_id);

--
-- Name: idx_github_connections_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_github_connections_agent ON public.github_connections USING btree (agent_id);

--
-- Name: idx_github_connections_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_github_connections_tenant ON public.github_connections USING btree (tenant_id);

--
-- Name: idx_github_events_conn; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_github_events_conn ON public.github_events USING btree (connection_id, created_at DESC);

--
-- Name: idx_goals_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_goals_agent ON public.goals USING btree (agent_id);

--
-- Name: idx_goals_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_goals_tenant ON public.goals USING btree (tenant_id);

--
-- Name: idx_hb_configs_next_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_hb_configs_next_run ON public.heartbeat_configs USING btree (next_run_at, enabled) WHERE (enabled = true);

--
-- Name: idx_hb_runs_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_hb_runs_agent ON public.heartbeat_runs USING btree (agent_id, ran_at DESC);

--
-- Name: idx_hb_runs_heartbeat; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_hb_runs_heartbeat ON public.heartbeat_runs USING btree (heartbeat_id, ran_at DESC);

--
-- Name: idx_inbound_rules_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_inbound_rules_agent ON public.inbound_rules USING btree (agent_id, priority);

--
-- Name: idx_key_usage_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_key_usage_agent ON public.key_usage_log USING btree (agent_id, created_at DESC);

--
-- Name: idx_key_usage_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_key_usage_key ON public.key_usage_log USING btree (key_id, created_at DESC);

--
-- Name: idx_kg_entities_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_entities_agent ON public.kg_entities USING btree (agent_id);

--
-- Name: idx_kg_entities_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_entities_tenant ON public.kg_entities USING btree (tenant_id);

--
-- Name: idx_kg_entities_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_entities_type ON public.kg_entities USING btree (tenant_id, entity_type);

--
-- Name: idx_kg_rels_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_rels_source ON public.kg_relationships USING btree (source_id);

--
-- Name: idx_kg_rels_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_rels_target ON public.kg_relationships USING btree (target_id);

--
-- Name: idx_kg_rels_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_kg_rels_tenant ON public.kg_relationships USING btree (tenant_id);

--
-- Name: idx_learned_skills_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_learned_skills_agent ON public.learned_skills USING btree (agent_id);

--
-- Name: idx_mail_approval; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_mail_approval ON public.mail_approval_queue USING btree (tenant_id, status, created_at);

--
-- Name: idx_mailbox_agent_folder; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_mailbox_agent_folder ON public.mailbox_messages USING btree (agent_id, folder, received_at DESC);

--
-- Name: idx_mailbox_thread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_mailbox_thread ON public.mailbox_messages USING btree (thread_id, received_at);

--
-- Name: idx_memdoc_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_memdoc_unique ON public.memory_documents USING btree (tenant_id, agent_id, COALESCE(user_id, ''::character varying), path);

--
-- Name: idx_memories_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_agent ON public.memories USING btree (tenant_id, agent_id);

--
-- Name: idx_memories_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_created_at ON public.memories USING btree (created_at DESC);

--
-- Name: idx_memories_importance; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_importance ON public.memories USING btree (agent_id, importance DESC);

--
-- Name: idx_memories_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_scope ON public.memories USING btree (scope, tenant_id);

--
-- Name: idx_memories_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_task ON public.memories USING btree (task_id) WHERE (task_id IS NOT NULL);

--
-- Name: idx_memories_tsv; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_tsv ON public.memories USING gin (tsv);

--
-- Name: idx_memories_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_type ON public.memories USING btree (agent_id, memory_type);

--
-- Name: idx_memories_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_updated_at ON public.memories USING btree (updated_at DESC);

--
-- Name: idx_memories_vec; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memories_vec ON public.memories USING hnsw (embedding public.vector_cosine_ops);

--
-- Name: idx_memory_chunks_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memory_chunks_agent ON public.memory_chunks USING btree (agent_id);

--
-- Name: idx_memory_chunks_tsv; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memory_chunks_tsv ON public.memory_chunks USING gin (tsv);

--
-- Name: idx_memory_edges_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memory_edges_source ON public.memory_edges USING btree (source_id);

--
-- Name: idx_memory_edges_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_memory_edges_target ON public.memory_edges USING btree (target_id);

--
-- Name: idx_notifications_tenant_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_notifications_tenant_agent ON public.notifications USING btree (tenant_id, agent_id);

--
-- Name: idx_notifications_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_notifications_user ON public.notifications USING btree (user_id, read, created_at DESC);

--
-- Name: idx_outbound_queue_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_outbound_queue_agent ON public.outbound_queue USING btree (agent_id);

--
-- Name: idx_outbound_queue_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_outbound_queue_status ON public.outbound_queue USING btree (status);

--
-- Name: idx_permission_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_permission_pending ON public.permission_requests USING btree (session_id, state) WHERE (state = 'pending'::text);

--
-- Name: idx_permission_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_permission_tenant ON public.permission_requests USING btree (tenant_id, state) WHERE (state = 'pending'::text);

--
-- Name: idx_pinned_tiles_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_pinned_tiles_tenant ON public.pinned_tiles USING btree (tenant_id, "position", created_at);

--
-- Name: idx_plan_edges_from; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plan_edges_from ON public.plan_edges USING btree (plan_id, from_node);

--
-- Name: idx_plan_nodes_assignee; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plan_nodes_assignee ON public.plan_nodes USING btree (assignee_soul);

--
-- Name: idx_plan_nodes_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plan_nodes_plan ON public.plan_nodes USING btree (plan_id);

--
-- Name: idx_plan_nodes_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plan_nodes_state ON public.plan_nodes USING btree (plan_id, state);

--
-- Name: idx_plans_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plans_active ON public.plans USING btree (tenant_id, created_at DESC) WHERE (archived_at IS NULL);

--
-- Name: idx_plans_archived; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plans_archived ON public.plans USING btree (tenant_id, archived_at DESC) WHERE (archived_at IS NOT NULL);

--
-- Name: idx_plans_project; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plans_project ON public.plans USING btree (project_id);

--
-- Name: idx_plans_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plans_session ON public.plans USING btree (session_id);

--
-- Name: idx_plans_tenant_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_plans_tenant_status ON public.plans USING btree (tenant_id, status);

--
-- Name: idx_prime_del_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_prime_del_status ON public.prime_delegations USING btree (status) WHERE (status = ANY (ARRAY['pending'::text, 'in_progress'::text]));

--
-- Name: idx_project_briefs_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_project_briefs_tenant ON public.project_briefs USING btree (tenant_id);

--
-- Name: idx_provider_keys_provider; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_provider_keys_provider ON public.provider_keys USING btree (tenant_id, provider_id);

--
-- Name: idx_providers_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_providers_tenant ON public.providers USING btree (tenant_id);

--
-- Name: idx_qoros_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_qoros_tenant ON public.qoros_state USING btree (tenant_id, active);

--
-- Name: idx_room_decisions_room; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_room_decisions_room ON public.room_decisions USING btree (room_id, status, created_at DESC);

--
-- Name: idx_room_messages_room; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_room_messages_room ON public.room_messages USING btree (room_id, created_at DESC);

--
-- Name: idx_room_minutes_room; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_room_minutes_room ON public.room_minutes USING btree (room_id, generated_at DESC);

--
-- Name: idx_room_tasks_assignee; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_room_tasks_assignee ON public.room_tasks USING btree (assigned_to, status) WHERE (status <> 'done'::text);

--
-- Name: idx_room_tasks_room; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_room_tasks_room ON public.room_tasks USING btree (room_id, status);

--
-- Name: idx_rooms_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_rooms_tenant ON public.rooms USING btree (tenant_id);

--
-- Name: idx_routing_decisions_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_routing_decisions_tenant ON public.routing_decisions USING btree (tenant_id, created_at DESC);

--
-- Name: idx_running_apps_prefix; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_running_apps_prefix ON public.running_apps USING btree (proxy_prefix);

--
-- Name: idx_running_apps_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_running_apps_session ON public.running_apps USING btree (session_id) WHERE (session_id IS NOT NULL);

--
-- Name: idx_running_apps_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_running_apps_tenant ON public.running_apps USING btree (tenant_id, status, created_at DESC);

--
-- Name: idx_sandbox_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sandbox_agent ON public.sandbox_runs USING btree (agent_id, created_at DESC);

--
-- Name: idx_scenarios_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_scenarios_tenant ON public.scenarios USING btree (tenant_id);

--
-- Name: idx_selected_models_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_selected_models_category ON public.selected_models USING btree (tenant_id, category);

--
-- Name: idx_selected_models_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_selected_models_tenant ON public.selected_models USING btree (tenant_id, is_default DESC, display_order);

--
-- Name: idx_service_accounts_active; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_service_accounts_active ON public.service_accounts USING btree (id) WHERE (revoked_at IS NULL);

--
-- Name: idx_service_accounts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_service_accounts_tenant ON public.service_accounts USING btree (tenant_id) WHERE ((tenant_id IS NOT NULL) AND (revoked_at IS NULL));

--
-- Name: idx_sessions_agent_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_agent_updated ON public.sessions USING btree (agent_id, updated_at DESC);

--
-- Name: idx_sessions_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON public.sessions USING btree (created_at DESC);

--
-- Name: idx_sessions_discussion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_discussion ON public.sessions USING btree (discussion_id);

--
-- Name: idx_sessions_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_owner ON public.sessions USING btree (owner_actor_id);

--
-- Name: idx_sessions_tenant_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_tenant_agent ON public.sessions USING btree (tenant_id, agent_id);

--
-- Name: idx_sessions_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sessions_updated ON public.sessions USING btree (updated_at DESC);

--
-- Name: idx_skills_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_skills_category ON public.skills USING btree (category);

--
-- Name: idx_skills_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_skills_slug ON public.skills USING btree (slug);

--
-- Name: idx_social_autoposts_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_autoposts_agent ON public.social_autoposts USING btree (agent_id);

--
-- Name: idx_social_integrations_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_integrations_agent ON public.social_integrations USING btree (agent_id);

--
-- Name: idx_social_integrations_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_integrations_platform ON public.social_integrations USING btree (platform);

--
-- Name: idx_social_posts_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_posts_agent ON public.social_posts USING btree (agent_id);

--
-- Name: idx_social_posts_status_when; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_posts_status_when ON public.social_posts USING btree (status, scheduled_at);

--
-- Name: idx_social_posts_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_social_posts_tenant ON public.social_posts USING btree (tenant_id);

--
-- Name: idx_soul_skills_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_soul_skills_agent ON public.soul_skills USING btree (agent_id);

--
-- Name: idx_soul_usage_soul; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_soul_usage_soul ON public.soul_usage USING btree (soul_id, called_at DESC);

--
-- Name: idx_soul_usage_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_soul_usage_tenant ON public.soul_usage USING btree (tenant_id, called_at DESC);

--
-- Name: idx_spans_trace; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_spans_trace ON public.spans USING btree (trace_id);

--
-- Name: idx_stream_ckpt_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_stream_ckpt_tenant ON public.stream_checkpoints USING btree (tenant_id);

--
-- Name: idx_subagent_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_subagent_parent ON public.subagent_runs USING btree (parent_agent_id);

--
-- Name: idx_subagent_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_subagent_status ON public.subagent_runs USING btree (status);

--
-- Name: idx_sup_reviews_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sup_reviews_agent ON public.supervisor_reviews USING btree (to_agent_id, status);

--
-- Name: idx_sup_reviews_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_sup_reviews_tenant ON public.supervisor_reviews USING btree (tenant_id, status, created_at DESC);

--
-- Name: idx_supervisor_messages_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_supervisor_messages_created ON public.supervisor_messages USING btree (created_at DESC);

--
-- Name: idx_supervisor_messages_exchange; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_supervisor_messages_exchange ON public.supervisor_messages USING btree (exchange_id);

--
-- Name: idx_supervisor_messages_intent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_supervisor_messages_intent ON public.supervisor_messages USING btree (intent);

--
-- Name: idx_task_comments_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_task_comments_task ON public.task_comments USING btree (task_id);

--
-- Name: idx_task_events_task_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_task_events_task_id ON public.task_events USING btree (task_id, created_at DESC);

--
-- Name: idx_tasks_assigned; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_assigned ON public.tasks USING btree (assigned_to, status);

--
-- Name: idx_tasks_discussion; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_discussion ON public.tasks USING btree (discussion_id) WHERE (discussion_id IS NOT NULL);

--
-- Name: idx_tasks_origin_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_origin_session ON public.tasks USING btree (origin_session_id) WHERE (origin_session_id IS NOT NULL);

--
-- Name: idx_tasks_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_parent ON public.tasks USING btree (parent_id);

--
-- Name: idx_tasks_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_state ON public.tasks USING btree (state);

--
-- Name: idx_tasks_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_status ON public.tasks USING btree (tenant_id, status);

--
-- Name: idx_tasks_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_tenant ON public.tasks USING btree (tenant_id);

--
-- Name: idx_tasks_ticket_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tasks_ticket_id ON public.tasks USING btree (ticket_id) WHERE (ticket_id IS NOT NULL);

--
-- Name: idx_ticket_comments_ticket; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_ticket_comments_ticket ON public.ticket_comments USING btree (ticket_id);

--
-- Name: idx_tickets_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tickets_agent ON public.tickets USING btree (assigned_agent_id);

--
-- Name: idx_tickets_brief; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tickets_brief ON public.tickets USING btree (project_brief_id);

--
-- Name: idx_tickets_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_tickets_tenant ON public.tickets USING btree (tenant_id);

--
-- Name: idx_traces_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_traces_tenant ON public.traces USING btree (tenant_id, created_at DESC);

--
-- Name: idx_user_presence_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_user_presence_tenant ON public.user_presence USING btree (tenant_id, is_online);

--
-- Name: idx_user_sessions_expires; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_user_sessions_expires ON public.user_sessions USING btree (expires_at);

--
-- Name: idx_user_sessions_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON public.user_sessions USING btree (user_id);

--
-- Name: idx_wa_pending_senders_channel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wa_pending_senders_channel ON public.whatsapp_pending_senders USING btree (channel_id);

--
-- Name: idx_wa_pending_senders_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wa_pending_senders_tenant ON public.whatsapp_pending_senders USING btree (tenant_id);

--
-- Name: idx_wakeup_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wakeup_agent ON public.wakeup_requests USING btree (agent_id, status, created_at);

--
-- Name: idx_wakeup_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wakeup_pending ON public.wakeup_requests USING btree (tenant_id, status, priority DESC) WHERE (status = 'pending'::text);

--
-- Name: idx_wakeup_plan; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wakeup_plan ON public.wakeup_requests USING btree (plan_id) WHERE (consumed_at IS NULL);

--
-- Name: idx_wakeup_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wakeup_tenant ON public.wakeup_requests USING btree (tenant_id, consumed_at) WHERE (consumed_at IS NULL);

--
-- Name: idx_wasm_plugins_tenant_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_wasm_plugins_tenant_created ON public.wasm_plugins USING btree (tenant_id, created_at DESC);

--
-- Name: idx_wasm_plugins_tenant_name_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS idx_wasm_plugins_tenant_name_active ON public.wasm_plugins USING btree (tenant_id, name) WHERE (revoked_at IS NULL);

--
-- Name: idx_work_goals_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_work_goals_parent ON public.work_goals USING btree (parent_id);

--
-- Name: idx_work_goals_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_work_goals_tenant ON public.work_goals USING btree (tenant_id);

--
-- Name: idx_workflow_runs_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_workflow_runs_status ON public.workflow_runs USING btree (status);

--
-- Name: idx_workflow_runs_workflow; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow ON public.workflow_runs USING btree (workflow_id);

--
-- Name: idx_workflows_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_workflows_agent ON public.workflows USING btree (agent_id);

--
-- Name: idx_workflows_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_workflows_tenant ON public.workflows USING btree (tenant_id);

--
-- Name: idx_ws_dash_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_ws_dash_tenant ON public.workspace_dashboards USING btree (tenant_id);

--
-- Name: magic_links_expires_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS magic_links_expires_idx ON public.magic_links USING btree (expires_at) WHERE (used = false);

--
-- Name: magic_links_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS magic_links_user_idx ON public.magic_links USING btree (user_id);

--
-- Name: media_providers_tenant_kind_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS media_providers_tenant_kind_idx ON public.media_providers USING btree (tenant_id, kind);

--
-- Name: refresh_tokens_active_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS refresh_tokens_active_idx ON public.refresh_tokens USING btree (token) WHERE (revoked_at IS NULL);

--
-- Name: refresh_tokens_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx ON public.refresh_tokens USING btree (user_id);

--
-- Name: task_files_task_path; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS task_files_task_path ON public.task_files USING btree (task_id, path);

--
-- Name: ticket_files_ticket_path; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS ticket_files_ticket_path ON public.ticket_files USING btree (ticket_id, path);

--
-- Name: tickets_tenant_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS tickets_tenant_slug ON public.tickets USING btree (tenant_id, slug);

--
-- Name: tool_approvals_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS tool_approvals_pending ON public.tool_approvals USING btree (agent_id, status) WHERE (status = 'pending'::text);

--
-- Name: uniq_approval_pending_per_node; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS uniq_approval_pending_per_node ON public.approvals USING btree (node_id) WHERE (state = 'pending'::text);

--
-- Name: uq_social_integrations_agent_platform; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX IF NOT EXISTS uq_social_integrations_agent_platform ON public.social_integrations USING btree (agent_id, platform) WHERE active;

--
-- Name: voice_providers_tenant_kind_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS voice_providers_tenant_kind_idx ON public.voice_providers USING btree (tenant_id, kind) WHERE (enabled = true);

--
-- Name: wakeup_dead_letter_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX IF NOT EXISTS wakeup_dead_letter_idx ON public.wakeup_requests USING btree (dead_letter_reason) WHERE (dead_letter_reason IS NOT NULL);

--
-- Name: agent_channel_bindings agent_channel_bindings_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_channel_bindings
    ADD CONSTRAINT agent_channel_bindings_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agent_messages agent_messages_from_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_from_agent_fkey FOREIGN KEY (from_agent) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agent_messages agent_messages_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE SET NULL;

--
-- Name: agent_messages agent_messages_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: agent_messages agent_messages_to_agent_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_messages
    ADD CONSTRAINT agent_messages_to_agent_fkey FOREIGN KEY (to_agent) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: agents agents_manager_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_manager_id_fkey FOREIGN KEY (manager_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: agents agents_project_brief_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_project_brief_id_fkey FOREIGN KEY (project_brief_id) REFERENCES public.project_briefs(id) ON DELETE SET NULL;

--
-- Name: agents agents_provider_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_provider_id_fkey FOREIGN KEY (provider_id) REFERENCES public.providers(id);

--
-- Name: agents agents_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: api_keys api_keys_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);

--
-- Name: approval_comments approval_comments_approval_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approval_comments
    ADD CONSTRAINT approval_comments_approval_id_fkey FOREIGN KEY (approval_id) REFERENCES public.approvals(id) ON DELETE CASCADE;

--
-- Name: approvals approvals_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_node_id_fkey FOREIGN KEY (node_id) REFERENCES public.plan_nodes(id) ON DELETE CASCADE;

--
-- Name: approvals approvals_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.approvals
    ADD CONSTRAINT approvals_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.plans(id) ON DELETE CASCADE;

--
-- Name: brief_agent_spend brief_agent_spend_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brief_agent_spend
    ADD CONSTRAINT brief_agent_spend_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: brief_agent_spend brief_agent_spend_brief_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brief_agent_spend
    ADD CONSTRAINT brief_agent_spend_brief_id_fkey FOREIGN KEY (brief_id) REFERENCES public.project_briefs(id) ON DELETE CASCADE;

--
-- Name: calendar_events calendar_events_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.calendar_events
    ADD CONSTRAINT calendar_events_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: channel_instances channel_instances_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_instances
    ADD CONSTRAINT channel_instances_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: channel_instances channel_instances_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.channel_instances
    ADD CONSTRAINT channel_instances_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: config_secrets config_secrets_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.config_secrets
    ADD CONSTRAINT config_secrets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: connector_actions connector_actions_platform_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_actions
    ADD CONSTRAINT connector_actions_platform_id_fkey FOREIGN KEY (platform_id) REFERENCES public.connector_platforms(id) ON DELETE CASCADE;

--
-- Name: connector_credentials connector_credentials_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.connector_credentials
    ADD CONSTRAINT connector_credentials_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: credentials credentials_platform_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.credentials
    ADD CONSTRAINT credentials_platform_id_fkey FOREIGN KEY (platform_id) REFERENCES public.connector_platforms(id) ON DELETE CASCADE;

--
-- Name: crew_members crew_members_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crew_members
    ADD CONSTRAINT crew_members_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: crew_members crew_members_crew_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crew_members
    ADD CONSTRAINT crew_members_crew_id_fkey FOREIGN KEY (crew_id) REFERENCES public.crews(id) ON DELETE CASCADE;

--
-- Name: crews crews_supervisor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crews
    ADD CONSTRAINT crews_supervisor_id_fkey FOREIGN KEY (supervisor_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: crews crews_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crews
    ADD CONSTRAINT crews_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: cron_jobs cron_jobs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cron_jobs
    ADD CONSTRAINT cron_jobs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: cron_jobs cron_jobs_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cron_jobs
    ADD CONSTRAINT cron_jobs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: crystallized_skills crystallized_skills_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crystallized_skills
    ADD CONSTRAINT crystallized_skills_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: crystallized_skills crystallized_skills_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.crystallized_skills
    ADD CONSTRAINT crystallized_skills_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.crystallized_skills(id);

--
-- Name: custom_tools custom_tools_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_tools
    ADD CONSTRAINT custom_tools_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: custom_tools custom_tools_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.custom_tools
    ADD CONSTRAINT custom_tools_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: daemon_agents daemon_agents_current_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_agents
    ADD CONSTRAINT daemon_agents_current_task_id_fkey FOREIGN KEY (current_task_id) REFERENCES public.daemon_tasks(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED;

--
-- Name: discussions discussions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: discussions discussions_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.discussions
    ADD CONSTRAINT discussions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: draft_replies draft_replies_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.draft_replies
    ADD CONSTRAINT draft_replies_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: draft_replies draft_replies_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.draft_replies
    ADD CONSTRAINT draft_replies_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.sessions(id) ON DELETE SET NULL;

--
-- Name: drive_files drive_files_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.drive_files
    ADD CONSTRAINT drive_files_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: drive_files drive_files_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.drive_files
    ADD CONSTRAINT drive_files_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.drive_files(id);

--
-- Name: drive_permissions drive_permissions_file_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.drive_permissions
    ADD CONSTRAINT drive_permissions_file_id_fkey FOREIGN KEY (file_id) REFERENCES public.drive_files(id) ON DELETE CASCADE;

--
-- Name: email_routing email_routing_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_routing
    ADD CONSTRAINT email_routing_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: daemon_tasks fk_daemon_tasks_owner; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_tasks
    ADD CONSTRAINT fk_daemon_tasks_owner FOREIGN KEY (owner) REFERENCES public.daemon_agents(id) ON DELETE SET NULL;

--
-- Name: daemon_tasks fk_daemon_tasks_plan; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.daemon_tasks
    ADD CONSTRAINT fk_daemon_tasks_plan FOREIGN KEY (plan_id) REFERENCES public.daemon_plans(id) ON DELETE SET NULL;

--
-- Name: github_events github_events_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_events
    ADD CONSTRAINT github_events_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES public.github_connections(id) ON DELETE CASCADE;

--
-- Name: goals goals_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.goals
    ADD CONSTRAINT goals_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: heartbeat_configs heartbeat_configs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_configs
    ADD CONSTRAINT heartbeat_configs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: heartbeat_configs heartbeat_configs_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_configs
    ADD CONSTRAINT heartbeat_configs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: heartbeat_queue heartbeat_queue_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_queue
    ADD CONSTRAINT heartbeat_queue_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: heartbeat_queue heartbeat_queue_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_queue
    ADD CONSTRAINT heartbeat_queue_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: heartbeat_runs heartbeat_runs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_runs
    ADD CONSTRAINT heartbeat_runs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: heartbeat_runs heartbeat_runs_heartbeat_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_runs
    ADD CONSTRAINT heartbeat_runs_heartbeat_id_fkey FOREIGN KEY (heartbeat_id) REFERENCES public.heartbeat_configs(id) ON DELETE CASCADE;

--
-- Name: heartbeat_runs heartbeat_runs_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.heartbeat_runs
    ADD CONSTRAINT heartbeat_runs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: inbound_agent_config inbound_agent_config_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_agent_config
    ADD CONSTRAINT inbound_agent_config_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: inbound_rules inbound_rules_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.inbound_rules
    ADD CONSTRAINT inbound_rules_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: key_usage_log key_usage_log_key_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.key_usage_log
    ADD CONSTRAINT key_usage_log_key_id_fkey FOREIGN KEY (key_id) REFERENCES public.provider_keys(id);

--
-- Name: kg_entities kg_entities_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_entities
    ADD CONSTRAINT kg_entities_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: kg_entities kg_entities_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_entities
    ADD CONSTRAINT kg_entities_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: kg_relationships kg_relationships_source_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_relationships
    ADD CONSTRAINT kg_relationships_source_id_fkey FOREIGN KEY (source_id) REFERENCES public.kg_entities(id) ON DELETE CASCADE;

--
-- Name: kg_relationships kg_relationships_target_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_relationships
    ADD CONSTRAINT kg_relationships_target_id_fkey FOREIGN KEY (target_id) REFERENCES public.kg_entities(id) ON DELETE CASCADE;

--
-- Name: kg_relationships kg_relationships_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.kg_relationships
    ADD CONSTRAINT kg_relationships_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: learned_skills learned_skills_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.learned_skills
    ADD CONSTRAINT learned_skills_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: magic_links magic_links_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_links
    ADD CONSTRAINT magic_links_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: mail_aliases mail_aliases_target_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_aliases
    ADD CONSTRAINT mail_aliases_target_agent_id_fkey FOREIGN KEY (target_agent_id) REFERENCES public.agents(id);

--
-- Name: mail_approval_queue mail_approval_queue_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_approval_queue
    ADD CONSTRAINT mail_approval_queue_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: mail_approval_queue mail_approval_queue_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_approval_queue
    ADD CONSTRAINT mail_approval_queue_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.mailbox_messages(id);

--
-- Name: mail_thread_assignments mail_thread_assignments_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mail_thread_assignments
    ADD CONSTRAINT mail_thread_assignments_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: mailbox_messages mailbox_messages_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_messages
    ADD CONSTRAINT mailbox_messages_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: mailbox_messages mailbox_messages_identity_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_messages
    ADD CONSTRAINT mailbox_messages_identity_id_fkey FOREIGN KEY (identity_id) REFERENCES public.soul_mail_identities(id);

--
-- Name: memories memories_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: memories memories_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: memory_bulletins memory_bulletins_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_bulletins
    ADD CONSTRAINT memory_bulletins_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: memory_bulletins memory_bulletins_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_bulletins
    ADD CONSTRAINT memory_bulletins_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: memory_documents memory_documents_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_documents
    ADD CONSTRAINT memory_documents_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: memory_documents memory_documents_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_documents
    ADD CONSTRAINT memory_documents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: memory_edges memory_edges_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memory_edges
    ADD CONSTRAINT memory_edges_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: model_discoveries model_discoveries_key_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.model_discoveries
    ADD CONSTRAINT model_discoveries_key_id_fkey FOREIGN KEY (key_id) REFERENCES public.provider_keys(id) ON DELETE CASCADE;

--
-- Name: paired_devices paired_devices_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.paired_devices
    ADD CONSTRAINT paired_devices_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: pairing_requests pairing_requests_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pairing_requests
    ADD CONSTRAINT pairing_requests_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: permission_requests permission_requests_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_requests
    ADD CONSTRAINT permission_requests_node_id_fkey FOREIGN KEY (node_id) REFERENCES public.plan_nodes(id) ON DELETE SET NULL;

--
-- Name: permission_requests permission_requests_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permission_requests
    ADD CONSTRAINT permission_requests_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.plans(id) ON DELETE SET NULL;

--
-- Name: plan_edges plan_edges_from_node_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_edges
    ADD CONSTRAINT plan_edges_from_node_fkey FOREIGN KEY (from_node) REFERENCES public.plan_nodes(id) ON DELETE CASCADE;

--
-- Name: plan_edges plan_edges_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_edges
    ADD CONSTRAINT plan_edges_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.plans(id) ON DELETE CASCADE;

--
-- Name: plan_edges plan_edges_to_node_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_edges
    ADD CONSTRAINT plan_edges_to_node_fkey FOREIGN KEY (to_node) REFERENCES public.plan_nodes(id) ON DELETE CASCADE;

--
-- Name: plan_nodes plan_nodes_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_nodes
    ADD CONSTRAINT plan_nodes_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.plan_nodes(id) ON DELETE CASCADE;

--
-- Name: plan_nodes plan_nodes_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plan_nodes
    ADD CONSTRAINT plan_nodes_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.plans(id) ON DELETE CASCADE;

--
-- Name: plans plans_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.plans
    ADD CONSTRAINT plans_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: prime_delegations prime_delegations_prime_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prime_delegations
    ADD CONSTRAINT prime_delegations_prime_id_fkey FOREIGN KEY (prime_id) REFERENCES public.agents(id);

--
-- Name: prime_delegations prime_delegations_specialist_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prime_delegations
    ADD CONSTRAINT prime_delegations_specialist_id_fkey FOREIGN KEY (specialist_id) REFERENCES public.agents(id);

--
-- Name: project_briefs project_briefs_goal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_briefs
    ADD CONSTRAINT project_briefs_goal_id_fkey FOREIGN KEY (goal_id) REFERENCES public.work_goals(id) ON DELETE SET NULL;

--
-- Name: project_briefs project_briefs_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.project_briefs
    ADD CONSTRAINT project_briefs_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: prompt_cache_stats prompt_cache_stats_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_cache_stats
    ADD CONSTRAINT prompt_cache_stats_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: providers providers_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.providers
    ADD CONSTRAINT providers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: refresh_tokens refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: room_decisions room_decisions_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_decisions
    ADD CONSTRAINT room_decisions_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.room_messages(id) ON DELETE SET NULL;

--
-- Name: room_decisions room_decisions_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_decisions
    ADD CONSTRAINT room_decisions_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE;

--
-- Name: room_members room_members_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_members
    ADD CONSTRAINT room_members_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: room_members room_members_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_members
    ADD CONSTRAINT room_members_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE;

--
-- Name: room_messages room_messages_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_messages
    ADD CONSTRAINT room_messages_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE;

--
-- Name: room_minutes room_minutes_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_minutes
    ADD CONSTRAINT room_minutes_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE;

--
-- Name: room_tasks room_tasks_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_tasks
    ADD CONSTRAINT room_tasks_message_id_fkey FOREIGN KEY (message_id) REFERENCES public.room_messages(id) ON DELETE SET NULL;

--
-- Name: room_tasks room_tasks_room_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.room_tasks
    ADD CONSTRAINT room_tasks_room_id_fkey FOREIGN KEY (room_id) REFERENCES public.rooms(id) ON DELETE CASCADE;

--
-- Name: running_apps running_apps_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.running_apps
    ADD CONSTRAINT running_apps_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: sandbox_runs sandbox_runs_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sandbox_runs
    ADD CONSTRAINT sandbox_runs_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: selected_models selected_models_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.selected_models
    ADD CONSTRAINT selected_models_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: service_accounts service_accounts_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_accounts
    ADD CONSTRAINT service_accounts_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE SET NULL;

--
-- Name: sessions sessions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: sessions sessions_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: skill_installations skill_installations_manifest_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_installations
    ADD CONSTRAINT skill_installations_manifest_id_fkey FOREIGN KEY (manifest_id) REFERENCES public.skill_manifests(id);

--
-- Name: skill_reviews skill_reviews_skill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skill_reviews
    ADD CONSTRAINT skill_reviews_skill_id_fkey FOREIGN KEY (skill_id) REFERENCES public.skills(id) ON DELETE CASCADE;

--
-- Name: skills skills_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.skills
    ADD CONSTRAINT skills_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: social_autoposts social_autoposts_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_autoposts
    ADD CONSTRAINT social_autoposts_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: social_autoposts social_autoposts_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_autoposts
    ADD CONSTRAINT social_autoposts_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: social_integrations social_integrations_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_integrations
    ADD CONSTRAINT social_integrations_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;

--
-- Name: social_integrations social_integrations_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_integrations
    ADD CONSTRAINT social_integrations_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: social_posts social_posts_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_posts
    ADD CONSTRAINT social_posts_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: social_posts social_posts_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.social_posts
    ADD CONSTRAINT social_posts_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: soul_mail_identities soul_mail_identities_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_mail_identities
    ADD CONSTRAINT soul_mail_identities_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: soul_skills soul_skills_skill_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_skills
    ADD CONSTRAINT soul_skills_skill_id_fkey FOREIGN KEY (skill_id) REFERENCES public.skills(id) ON DELETE CASCADE;

--
-- Name: soul_usage soul_usage_soul_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.soul_usage
    ADD CONSTRAINT soul_usage_soul_id_fkey FOREIGN KEY (soul_id) REFERENCES public.agents(id);

--
-- Name: spans spans_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spans
    ADD CONSTRAINT spans_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: spans spans_trace_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spans
    ADD CONSTRAINT spans_trace_id_fkey FOREIGN KEY (trace_id) REFERENCES public.traces(id) ON DELETE CASCADE;

--
-- Name: subagent_runs subagent_runs_child_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subagent_runs
    ADD CONSTRAINT subagent_runs_child_agent_id_fkey FOREIGN KEY (child_agent_id) REFERENCES public.agents(id);

--
-- Name: subagent_runs subagent_runs_parent_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.subagent_runs
    ADD CONSTRAINT subagent_runs_parent_agent_id_fkey FOREIGN KEY (parent_agent_id) REFERENCES public.agents(id);

--
-- Name: supervisor_messages supervisor_messages_exchange_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supervisor_messages
    ADD CONSTRAINT supervisor_messages_exchange_id_fkey FOREIGN KEY (exchange_id) REFERENCES public.supervisor_exchanges(id);

--
-- Name: task_comments task_comments_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_comments
    ADD CONSTRAINT task_comments_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

--
-- Name: task_events task_events_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_events
    ADD CONSTRAINT task_events_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: task_events task_events_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_events
    ADD CONSTRAINT task_events_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

--
-- Name: task_files task_files_task_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.task_files
    ADD CONSTRAINT task_files_task_id_fkey FOREIGN KEY (task_id) REFERENCES public.tasks(id) ON DELETE CASCADE;

--
-- Name: tasks tasks_assigned_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_assigned_by_fkey FOREIGN KEY (assigned_by) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: tasks tasks_assigned_to_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: tasks tasks_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.tasks(id) ON DELETE SET NULL;

--
-- Name: tasks tasks_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: tasks tasks_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE SET NULL;

--
-- Name: team_task_attachments team_task_attachments_created_by_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_attachments
    ADD CONSTRAINT team_task_attachments_created_by_agent_id_fkey FOREIGN KEY (created_by_agent_id) REFERENCES public.agents(id);

--
-- Name: team_task_attachments team_task_attachments_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_attachments
    ADD CONSTRAINT team_task_attachments_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.crews(id);

--
-- Name: team_task_comments team_task_comments_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_task_comments
    ADD CONSTRAINT team_task_comments_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: ticket_comments ticket_comments_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_comments
    ADD CONSTRAINT ticket_comments_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: ticket_counters ticket_counters_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_counters
    ADD CONSTRAINT ticket_counters_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: ticket_files ticket_files_ticket_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ticket_files
    ADD CONSTRAINT ticket_files_ticket_id_fkey FOREIGN KEY (ticket_id) REFERENCES public.tickets(id) ON DELETE CASCADE;

--
-- Name: tickets tickets_assigned_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_assigned_agent_id_fkey FOREIGN KEY (assigned_agent_id) REFERENCES public.agents(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_goal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_goal_id_fkey FOREIGN KEY (goal_id) REFERENCES public.work_goals(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_project_brief_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_project_brief_id_fkey FOREIGN KEY (project_brief_id) REFERENCES public.project_briefs(id) ON DELETE SET NULL;

--
-- Name: tickets tickets_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.tickets
    ADD CONSTRAINT tickets_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: traces traces_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.traces
    ADD CONSTRAINT traces_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: user_presence user_presence_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_presence
    ADD CONSTRAINT user_presence_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: user_presence user_presence_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_presence
    ADD CONSTRAINT user_presence_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: user_sessions user_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_sessions
    ADD CONSTRAINT user_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

--
-- Name: voice_providers voice_providers_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voice_providers
    ADD CONSTRAINT voice_providers_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: wakeup_requests wakeup_requests_node_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wakeup_requests
    ADD CONSTRAINT wakeup_requests_node_id_fkey FOREIGN KEY (node_id) REFERENCES public.plan_nodes(id) ON DELETE CASCADE;

--
-- Name: wakeup_requests wakeup_requests_plan_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wakeup_requests
    ADD CONSTRAINT wakeup_requests_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES public.plans(id) ON DELETE CASCADE;

--
-- Name: wasm_plugins wasm_plugins_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.wasm_plugins
    ADD CONSTRAINT wasm_plugins_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: whatsapp_pending_senders whatsapp_pending_senders_channel_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.whatsapp_pending_senders
    ADD CONSTRAINT whatsapp_pending_senders_channel_id_fkey FOREIGN KEY (channel_id) REFERENCES public.channel_instances(id) ON DELETE CASCADE;

--
-- Name: work_goals work_goals_parent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_goals
    ADD CONSTRAINT work_goals_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES public.work_goals(id) ON DELETE CASCADE;

--
-- Name: work_goals work_goals_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.work_goals
    ADD CONSTRAINT work_goals_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: workflow_runs workflow_runs_workflow_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflow_runs
    ADD CONSTRAINT workflow_runs_workflow_id_fkey FOREIGN KEY (workflow_id) REFERENCES public.workflows(id) ON DELETE CASCADE;

--
-- Name: workflows workflows_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflows
    ADD CONSTRAINT workflows_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id);

--
-- Name: workflows workflows_tenant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.workflows
    ADD CONSTRAINT workflows_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES public.tenants(id) ON DELETE CASCADE;

--
-- Name: agent_messages; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.agent_messages ENABLE ROW LEVEL SECURITY;

--
-- Name: agents; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.agents ENABLE ROW LEVEL SECURITY;

--
-- Name: approvals; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.approvals ENABLE ROW LEVEL SECURITY;

--
-- Name: apps; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.apps ENABLE ROW LEVEL SECURITY;

--
-- Name: channel_instances; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.channel_instances ENABLE ROW LEVEL SECURITY;

--
-- Name: config_secrets; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.config_secrets ENABLE ROW LEVEL SECURITY;

--
-- Name: cron_jobs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.cron_jobs ENABLE ROW LEVEL SECURITY;

--
-- Name: heartbeat_configs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.heartbeat_configs ENABLE ROW LEVEL SECURITY;

--
-- Name: heartbeat_runs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.heartbeat_runs ENABLE ROW LEVEL SECURITY;

--
-- Name: kg_entities; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.kg_entities ENABLE ROW LEVEL SECURITY;

--
-- Name: kg_relationships; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.kg_relationships ENABLE ROW LEVEL SECURITY;

--
-- Name: memories; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.memories ENABLE ROW LEVEL SECURITY;

--
-- Name: memory_bulletins; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.memory_bulletins ENABLE ROW LEVEL SECURITY;

--
-- Name: memory_chunks; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.memory_chunks ENABLE ROW LEVEL SECURITY;

--
-- Name: memory_documents; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.memory_documents ENABLE ROW LEVEL SECURITY;

--
-- Name: memory_edges; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.memory_edges ENABLE ROW LEVEL SECURITY;

--
-- Name: paired_devices; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.paired_devices ENABLE ROW LEVEL SECURITY;

--
-- Name: permission_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.permission_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: plan_edges; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.plan_edges ENABLE ROW LEVEL SECURITY;

--
-- Name: plan_nodes; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.plan_nodes ENABLE ROW LEVEL SECURITY;

--
-- Name: plans; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.plans ENABLE ROW LEVEL SECURITY;

--
-- Name: providers; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.providers ENABLE ROW LEVEL SECURITY;

--
-- Name: apps rls_apps; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_apps ON public.apps USING ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant())));

--
-- Name: agent_messages rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.agent_messages USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: agents rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.agents USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: approvals rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.approvals USING ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = approvals.plan_id) AND (plans.tenant_id = public.app_current_tenant())))))) WITH CHECK ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = approvals.plan_id) AND (plans.tenant_id = public.app_current_tenant()))))));

--
-- Name: channel_instances rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.channel_instances USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: config_secrets rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.config_secrets USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: cron_jobs rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.cron_jobs USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: heartbeat_configs rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.heartbeat_configs USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: heartbeat_runs rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.heartbeat_runs USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: kg_entities rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.kg_entities USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: kg_relationships rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.kg_relationships USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: memories rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.memories USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: memory_bulletins rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.memory_bulletins USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: memory_chunks rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.memory_chunks USING ((public.app_rls_bypass() OR (tenant_id = NULLIF(current_setting('app.tenant_id'::text, true), ''::text)))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = NULLIF(current_setting('app.tenant_id'::text, true), ''::text))));

--
-- Name: memory_documents rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.memory_documents USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: memory_edges rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.memory_edges USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: paired_devices rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.paired_devices USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: permission_requests rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.permission_requests USING ((public.app_rls_bypass() OR ((tenant_id)::uuid = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR ((tenant_id)::uuid = public.app_current_tenant())));

--
-- Name: plan_edges rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.plan_edges USING ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = plan_edges.plan_id) AND (plans.tenant_id = public.app_current_tenant())))))) WITH CHECK ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = plan_edges.plan_id) AND (plans.tenant_id = public.app_current_tenant()))))));

--
-- Name: plan_nodes rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.plan_nodes USING ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = plan_nodes.plan_id) AND (plans.tenant_id = public.app_current_tenant())))))) WITH CHECK ((public.app_rls_bypass() OR (EXISTS ( SELECT 1
   FROM public.plans
  WHERE ((plans.id = plan_nodes.plan_id) AND (plans.tenant_id = public.app_current_tenant()))))));

--
-- Name: plans rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.plans USING ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant())));

--
-- Name: providers rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.providers USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: service_accounts rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.service_accounts USING ((public.app_rls_bypass() OR (tenant_id IS NULL) OR (tenant_id = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id IS NULL) OR (tenant_id = public.app_current_tenant())));

--
-- Name: sessions rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.sessions USING ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant())));

--
-- Name: skills rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.skills USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: spans rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.spans USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: tasks rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.tasks USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: traces rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.traces USING ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = (NULLIF(current_setting('app.tenant_id'::text, true), ''::text))::uuid)));

--
-- Name: wakeup_requests rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.wakeup_requests USING ((public.app_rls_bypass() OR ((tenant_id)::uuid = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR ((tenant_id)::uuid = public.app_current_tenant())));

--
-- Name: wasm_plugins rls_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY rls_tenant_isolation ON public.wasm_plugins USING ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant()))) WITH CHECK ((public.app_rls_bypass() OR (tenant_id = public.app_current_tenant())));

--
-- Name: service_accounts; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.service_accounts ENABLE ROW LEVEL SECURITY;

--
-- Name: sessions; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.sessions ENABLE ROW LEVEL SECURITY;

--
-- Name: skills; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.skills ENABLE ROW LEVEL SECURITY;

--
-- Name: spans; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.spans ENABLE ROW LEVEL SECURITY;

--
-- Name: tasks; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.tasks ENABLE ROW LEVEL SECURITY;


--
-- Name: traces; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.traces ENABLE ROW LEVEL SECURITY;

--
-- Name: wakeup_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.wakeup_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: wasm_plugins; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.wasm_plugins ENABLE ROW LEVEL SECURITY;

--
--


-- ── Post-baseline additions (folded in for clean fresh-install) ───────────────

-- Mail policy on agents (083)
ALTER TABLE public.agents
    ADD COLUMN IF NOT EXISTS mail_policy TEXT NOT NULL DEFAULT '';

-- Company + email fields on contacts (083)
ALTER TABLE public.contacts
    ADD COLUMN IF NOT EXISTS company TEXT,
    ADD COLUMN IF NOT EXISTS email TEXT GENERATED ALWAYS AS (
        CASE WHEN channel = 'email' THEN external_id ELSE NULL END
    ) STORED;

-- Status tracking on inbound_rules (083)
ALTER TABLE public.inbound_rules
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'pending_confirmation')),
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';

-- Per-agent enrichment of workspace contacts (083)
CREATE TABLE IF NOT EXISTS public.contact_agent_prefs (
    contact_id   UUID        NOT NULL REFERENCES public.contacts(id) ON DELETE CASCADE,
    agent_id     UUID        NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
    routing_mode TEXT        NOT NULL DEFAULT 'inherit'
        CHECK (routing_mode IN ('inherit', 'auto', 'draft', 'skip')),
    trust_level  TEXT        NOT NULL DEFAULT 'unknown'
        CHECK (trust_level IN ('unknown', 'known', 'trusted', 'blocked')),
    agent_notes  TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (contact_id, agent_id)
);
CREATE INDEX IF NOT EXISTS idx_contact_agent_prefs_agent
    ON public.contact_agent_prefs (agent_id);

-- Encrypted mail identity passwords (084)
ALTER TABLE public.soul_mail_identities
    ADD COLUMN IF NOT EXISTS smtp_pass_enc TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS imap_pass_enc TEXT NOT NULL DEFAULT '';

-- Refresh token IP tracking + user avatar (085)
ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS ip_address   TEXT,
    ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS avatar_url TEXT DEFAULT '' NOT NULL;

-- Tool permission policies (086)
CREATE TABLE IF NOT EXISTS permission_policies (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL,
    user_id    UUID        NOT NULL,
    tool       TEXT        NOT NULL,
    decision   TEXT        NOT NULL DEFAULT 'allow' CHECK (decision IN ('allow')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, tool)
);
CREATE INDEX IF NOT EXISTS idx_permission_policies_lookup
    ON permission_policies (tenant_id, user_id, tool);

-- Per-agent permission scope (087)
ALTER TABLE permission_policies
    ADD COLUMN IF NOT EXISTS agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS scope    TEXT NOT NULL DEFAULT 'auto_approved'
        CHECK (scope IN ('auto_approved', 'ask_first', 'blocked'));
ALTER TABLE permission_policies
    DROP CONSTRAINT IF EXISTS permission_policies_tenant_id_user_id_tool_key;
ALTER TABLE permission_policies
    ADD CONSTRAINT permission_policies_unique
        UNIQUE (tenant_id, user_id, tool, agent_id);
CREATE INDEX IF NOT EXISTS idx_permission_policies_agent
    ON permission_policies (tenant_id, agent_id, tool);

-- Cron job execution tracking columns (088)
ALTER TABLE cron_jobs
    ADD COLUMN IF NOT EXISTS executor_agent_id UUID,
    ADD COLUMN IF NOT EXISTS delivery_channel  TEXT NOT NULL DEFAULT 'web',
    ADD COLUMN IF NOT EXISTS run_count         INT  NOT NULL DEFAULT 0;

-- Seed built-in service accounts (from migration 037)
INSERT INTO public.service_accounts (id, role, description, created_by, revoked_at)
VALUES
    ('system',       'service',       'Internal platform system account',   'system', NULL),
    ('orchestrator', 'orchestrator',  'Multi-agent orchestration account',  'system', NULL),
    ('qoros',        'service',       'Qoros workspace service account',    'system', NULL)
ON CONFLICT (id) DO NOTHING;

-- User profile store: facts the agent knows about the user (name, preferences, language, timezone).
-- Injected into system prompt via InjectUserProfile() on every session.
CREATE TABLE IF NOT EXISTS public.user_profiles (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   UUID        NOT NULL UNIQUE,
    facts       JSONB       NOT NULL DEFAULT '{}',
    preferences JSONB       NOT NULL DEFAULT '{}',
    timezone    TEXT        NOT NULL DEFAULT 'UTC',
    language    TEXT        NOT NULL DEFAULT 'en',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Mark baseline version so the migration runner knows we're up to date
INSERT INTO schema_migrations (version, dirty) VALUES (1, false)
ON CONFLICT (version) DO NOTHING;
