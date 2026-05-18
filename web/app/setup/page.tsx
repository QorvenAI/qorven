'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import { setToken, isAuthenticated } from '@/lib/api';
import { ArrowLeft, ArrowRight, Check } from 'lucide-react';

import {
  TOTAL_STEPS, toVisualStep,
  PROVIDER_OPTIONS_FALLBACK, PROVIDER_TYPE_OVERRIDES,
  RECOMMENDED_PRIMARY, RECOMMENDED_FAST, RECOMMENDED_CODING,
  SPECIALIST_PRESETS,
  type ProviderOption, type Provider, type AgentSummary,
  type ProviderManifest, type AddedProvider,
} from '@/components/setup/setup-config';
import { api, listAgents } from '@/components/setup/setup-api';
import { QorvenSpinner } from '@/components/setup/setup-atoms';
import { CapabilitiesNotice }  from '@/components/setup/capabilities-notice';
import { Step1Admin }          from '@/components/setup/step1-admin';
import { Step2Workspace }      from '@/components/setup/step2-workspace';
import { Step4Provider }       from '@/components/setup/step4-provider';
import { Step7Channels }       from '@/components/setup/step7-channels';
import { Step9Summary }        from '@/components/setup/step9-summary';

export default function SetupPage() {
  const router = useRouter();
  const [step, setStep] = useState(1);
  const [bootstrapping, setBootstrapping] = useState(true);
  const [appVersion, setAppVersion] = useState('');
  const [setupRequired, setSetupRequired] = useState<boolean | null>(null);
  const [adminCreated, setAdminCreated] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [globalError, setGlobalError] = useState<string | null>(null);

  // Step 1 — admin
  const [displayName, setDisplayName] = useState('');
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPw, setShowPw] = useState(false);
  const [creatingAdmin, setCreatingAdmin] = useState(false);

  // Step 2 — Prime name + avatar + user persona
  const [primeName, setPrimeName] = useState('Prime');
  const [primeGradient, setPrimeGradient] = useState(0);
  const [callMe, setCallMe] = useState('');
  const [style, setStyle] = useState('casual');
  const [language, setLanguage] = useState('en');

  // Step 4 — LLM providers (multi-provider)
  const [catalog, setCatalog] = useState<ProviderOption[]>(PROVIDER_OPTIONS_FALLBACK);
  const [addedProviders, setAddedProviders] = useState<AddedProvider[]>([]);
  const [selectedOption, setSelectedOption] = useState<string>('bedrock');
  const [region, setRegion] = useState('us-east-1');
  const [apiKey, setApiKey] = useState('');
  const [apiBase, setApiBase] = useState('');
  const [awsAccessKey, setAwsAccessKey] = useState('');
  const [awsSecretKey, setAwsSecretKey] = useState('');
  const [bedrockProviderID, setBedrockProviderID] = useState<string | null>(null);
  const [providerStatus, setProviderStatus] = useState<'idle'|'creating'|'testing'|'ok'|'error'>('idle');
  const [providerError, setProviderError] = useState('');
  const [providerSample, setProviderSample] = useState('');
  const [bedrockModels, setBedrockModels] = useState<string[]>([]);
  const [primaryModel, setPrimaryModel] = useState(RECOMMENDED_PRIMARY);
  const [fastModel, setFastModel]       = useState(RECOMMENDED_FAST);
  const [codingModel, setCodingModel]   = useState(RECOMMENDED_CODING);

  // Step 5 — smart router
  const [routingAssignments, setRoutingAssignments] = useState<Record<string,string>>({});
  const [routerSaving, setRouterSaving] = useState(false);
  const [routerSaved, setRouterSaved] = useState(false);

  // Step 6 — specialists
  const [selectedSpecialists, setSelectedSpecialists] = useState<string[]>([]);
  const [creatingSpecialists, setCreatingSpecialists] = useState(false);
  const [specialistsCreated, setSpecialistsCreated] = useState(0);

  // Step 7 — channels
  const [telegramToken, setTelegramToken] = useState('');
  const [connectedChannels, setConnectedChannels] = useState<string[]>([]);
  const [channelBusy, setChannelBusy] = useState<string | null>(null);
  const [channelError, setChannelError] = useState<string | null>(null);


  // Step 8 — test chat
  const [primeID, setPrimeID] = useState<string | null>(null);

  // ── Bootstrap ─────────────────────────────────────────────────────────────
  useEffect(() => {
    (async () => {
      fetch('/api/health').then(r => r.json()).then((d: { version?: string }) => {
        if (d.version) setAppVersion(d.version);
      }).catch(() => {});

      try {
        const r = await api<{ setup_required: boolean }>('/auth/setup-check');
        setSetupRequired(r.setup_required);
        setAdminCreated(!r.setup_required);
        if (!r.setup_required) { setAccepted(true); setStep(2); }

        if (!r.setup_required && isAuthenticated()) {
          const agentList = await listAgents();
          const prime = agentList.find(a => a.agent_key === 'chief') ?? agentList.find(a => a.agent_key === 'prime');
          if (prime) setPrimeID(prime.id);
        }
      } catch (e) {
        setGlobalError(e instanceof Error ? e.message : 'Setup check failed');
      } finally {
        setBootstrapping(false);
      }
    })();
  }, []);

  // ── Fetch provider catalog ────────────────────────────────────────────────
  useEffect(() => {
    api<ProviderManifest[]>('/v1/providers/catalog')
      .then(list => {
        if (!Array.isArray(list) || !list.length) return;
        const mapped: ProviderOption[] = list
          .filter(m => !['search', 'voice', 'data', 'embeddings', 'media'].includes(m.category ?? ''))
          .map(m => {
            const fields: ProviderOption['fields'] = [];
            if (m.auth_type === 'aws_credentials') {
              fields.push('region');
              fields.push('aws_access_key');
            } else if (m.fields?.some(f => f.name === 'api_key')) {
              fields.push('api_key');
            }
            if (m.fields?.some(f => f.name === 'api_base')) fields.push('api_base');
            if (fields.length === 0 && m.auth_type !== 'none') fields.push('api_key');
            const override = PROVIDER_TYPE_OVERRIDES[m.id];
            const providerType = override ?? (
              m.auth_type === 'aws_credentials' ? 'bedrock' :
              m.id === 'anthropic' ? 'anthropic_native' :
              m.id === 'gemini' ? 'gemini_native' :
              m.id === 'dashscope' ? 'dashscope' :
              'openai_compat'
            );
            return {
              id: m.id,
              label: m.name,
              hint: '',
              providerType,
              fields,
              defaultApiBase: m.default_api_base,
              category: (m.category === 'cloud' ? 'cloud' : m.category === 'local' ? 'local' : 'openai_compat') as ProviderOption['category'],
            };
          });
        if (mapped.length) setCatalog(mapped);
      })
      .catch(() => { /* keep fallback */ });
  }, []);

  // ── Step 1 — create admin ─────────────────────────────────────────────────
  async function handleCreateAdmin() {
    setCreatingAdmin(true); setGlobalError(null);
    try {
      if (!username) throw new Error('Username required');
      if (password.length < 8) throw new Error('Password must be at least 8 characters');
      await api('/auth/setup', { method: 'POST', body: JSON.stringify({ username, password, display_name: displayName, email: email || undefined }) });
      const login = await api<{ token: string }>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
      setToken(login.token);
      setAdminCreated(true);
      setStep(2);
    } catch (e) {
      setGlobalError(e instanceof Error ? e.message : 'Admin setup failed');
    } finally {
      setCreatingAdmin(false);
    }
  }

  // ── Step 4 — test and persist provider ───────────────────────────────────
  async function setupProvider() {
    const opt = catalog.find(o => o.id === selectedOption) ?? catalog[0];
    if (!opt) { setProviderError('unknown provider'); return; }

    setProviderStatus('testing'); setProviderError(''); setProviderSample('');
    try {
      const effectiveProviderType = opt.providerType;
      const testBody: Record<string, unknown> = { name: opt.id, provider_type: effectiveProviderType };
      if (opt.fields.includes('region'))          testBody.region    = region;
      if (opt.fields.includes('api_key'))         testBody.api_key   = apiKey;
      if (opt.fields.includes('api_base'))        testBody.api_base  = apiBase || opt.defaultApiBase || '';
      else                                         testBody.api_base  = opt.defaultApiBase || '';
      if (opt.fields.includes('aws_access_key') && awsAccessKey) {
        testBody.aws_access_key = awsAccessKey;
        testBody.aws_secret_key = awsSecretKey;
      }

      const testRes = await api<{ success: boolean; sample?: string; models?: string[]; error?: string }>(
        '/v1/providers/test', { method: 'POST', body: JSON.stringify(testBody) });
      if (!testRes.success) throw new Error(testRes.error || 'provider test failed');
      setProviderSample(testRes.sample ?? '');

      const { providers: list } = await api<{ providers: Provider[] }>('/v1/providers');
      const existing = Array.isArray(list) ? list.find(p => p.name === opt.id) : null;
      let persisted: Provider;
      if (existing) {
        persisted = existing;
      } else {
        const createBody: Record<string, unknown> = {
          name: opt.id, display_name: opt.label, provider_type: effectiveProviderType,
          api_base: (testBody.api_base as string),
          api_key: opt.fields.includes('api_key') ? apiKey : '',
          enabled: true, settings: {},
        };
        if (opt.fields.includes('aws_access_key') && awsAccessKey) {
          createBody.aws_access_key = awsAccessKey;
          createBody.aws_secret_key = awsSecretKey;
        }
        persisted = await api<Provider>('/v1/providers', { method: 'POST', body: JSON.stringify(createBody) });
      }
      setBedrockProviderID(persisted.id);

      let models = testRes.models || [];
      if (!models.length) {
        const mm = await api<{ models: string[] }>(`/v1/providers/${persisted.id}/models`).catch(() => ({ models: [] }));
        models = mm.models ?? [];
      }
      setBedrockModels(models);

      if (opt.id === 'bedrock') {
        setPrimaryModel(RECOMMENDED_PRIMARY);
        setFastModel(RECOMMENDED_FAST);
        setCodingModel(RECOMMENDED_CODING);
      } else if (models.length > 0) {
        setPrimaryModel(models[0] ?? '');
        setFastModel(models[models.length > 1 ? 1 : 0] ?? '');
        setCodingModel(models[models.length > 2 ? 2 : 0] ?? '');
      }

      setProviderStatus('ok');
      setAddedProviders(prev => {
        if (prev.some(p => p.id === opt.id)) return prev;
        return [...prev, { id: opt.id, name: opt.label, providerDbId: persisted.id }];
      });
    } catch (e) {
      setProviderStatus('error');
      setProviderError(e instanceof Error ? e.message : 'provider setup failed');
    }
  }

  // ── Step 2 — update Prime ─────────────────────────────────────────────────
  async function persistPrime(): Promise<string | null> {
    try {
      const agentList = await listAgents();
      const prime = agentList.find(a => a.agent_key === 'chief') ?? agentList.find(a => a.agent_key === 'prime');
      if (!prime) return null;

      const systemPrompt = `You are ${primeName}, a personal AI assistant. You can handle any task — from writing code to answering questions, planning, research, and more. Be helpful, clear, and direct.`;

      const body: Record<string, unknown> = { display_name: primeName, system_prompt: systemPrompt, model: primaryModel };
      if (bedrockProviderID) body.provider_id = bedrockProviderID;

      await api(`/v1/agents/${prime.id}`, { method: 'PUT', body: JSON.stringify(body) });
      setPrimeID(prime.id);
      return prime.id;
    } catch {
      return null;
    }
  }

  // ── Step 6 — create specialist agents ────────────────────────────────────
  async function createSpecialists() {
    setCreatingSpecialists(true);
    let count = 0;
    for (const key of selectedSpecialists) {
      const preset = SPECIALIST_PRESETS.find(p => p.key === key);
      if (!preset) continue;
      try {
        await api('/v1/agents', {
          method: 'POST',
          body: JSON.stringify({
            agent_key: `seed_${preset.key}`,
            display_name: preset.display_name,
            role: preset.role,
            title: preset.title,
            model: preset.model,
            system_prompt: preset.system_prompt,
            temperature: preset.temperature,
            context_window: 128000,
            tool_profile: 'full',
            memory_enabled: true,
            outbound_approval: 'none',
            provider_id: bedrockProviderID,
          }),
        });
        count += 1;
      } catch { /* already exists, skip */ }
    }
    setSpecialistsCreated(count);
    setCreatingSpecialists(false);
  }

  // ── Step 7 — connect Telegram ─────────────────────────────────────────────
  async function connectChannel(type: 'telegram') {
    setChannelBusy(type); setChannelError(null);
    try {
      let resolvedPrimeID = primeID;
      if (!resolvedPrimeID) resolvedPrimeID = await persistPrime();
      if (!resolvedPrimeID) {
        const agentList = await listAgents();
        const chief = agentList.find(a => a.agent_key === 'chief') ?? agentList.find(a => a.agent_key === 'prime');
        resolvedPrimeID = chief?.id ?? null;
      }
      if (!resolvedPrimeID) throw new Error('Prime agent not found — go back to step 4 and add a provider first.');
      if (!telegramToken) throw new Error('Telegram bot token required');
      const ch = await api<{ id: string }>('/v1/channels', { method: 'POST', body: JSON.stringify({ agent_id: resolvedPrimeID, channel_type: 'telegram', name: 'telegram-main', config: { bot_token: telegramToken } }) });
      await api(`/v1/channels/${ch.id}/start`, { method: 'POST' }).catch(() => {});
      setConnectedChannels(prev => prev.includes(type) ? prev : [...prev, type]);
    } catch (e) {
      setChannelError(e instanceof Error ? e.message : 'Channel setup failed');
    } finally {
      setChannelBusy(null);
    }
  }

  // ── Final step — finalize + redirect ─────────────────────────────────────
  async function finalise() {
    try {
      await api('/v1/setup/finalize', {
        method: 'POST',
        body: JSON.stringify({
          instance_name: 'My Workspace',
          prime_name: primeName,
          call_me: callMe || displayName,
          style: style || 'casual',
          language: language || 'en',
        }),
      }).catch(() => {});
    } finally {
      router.push('/');
    }
  }

  // ── Navigation ────────────────────────────────────────────────────────────
  async function goNext() {
    setGlobalError(null);
    if (step === 3 && addedProviders.length > 0) { await persistPrime(); }
    if (step === 4 && !primeID) { await persistPrime(); }
    if (step < TOTAL_STEPS) setStep(step + 1);
  }
  function goBack() { setGlobalError(null); if (step > 1) setStep(step - 1); }

  function isNextDisabled(): boolean {
    if (step === 1) return !adminCreated;
    if (step === 3) return addedProviders.length === 0;
    return false;
  }

  // ── Bootstrap loading screen ──────────────────────────────────────────────
  if (bootstrapping) {
    return (
      <div className="min-h-screen w-full bg-background flex">
        <div className="hidden lg:flex w-[320px] shrink-0 flex-col bg-gradient-to-br from-violet-950 via-violet-900 to-fuchsia-900 px-10 py-12 relative overflow-hidden">
          <div className="absolute -top-16 -right-16 w-72 h-72 rounded-full bg-fuchsia-500/20 blur-3xl pointer-events-none" />
          <div className="absolute bottom-0 left-0 w-48 h-48 rounded-full bg-violet-400/10 blur-3xl pointer-events-none" />
          <div className="relative z-10 flex flex-col h-full">
            <div className="mb-12"><img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-9" /></div>
            <div className="flex-1" />
            <div className="space-y-0.5">
              <p className="text-xs text-white/30">qorven.ai</p>
              {appVersion && <p className="text-xs text-white/50">Version {appVersion}</p>}
              <p className="text-xs text-white/20">&copy; 2026 Qorven AI</p>
            </div>
          </div>
        </div>
        <div className="grow flex flex-col items-center justify-center gap-4">
          <QorvenSpinner className="h-10 w-10 opacity-70" />
          <p className="text-sm text-muted-foreground">Starting up…</p>
        </div>
      </div>
    );
  }

  const visualStep = toVisualStep(step);

  return (
    <div className="h-screen w-full bg-background flex overflow-hidden">
      {/* Left panel */}
      <div className="hidden lg:flex w-[300px] shrink-0 flex-col bg-gradient-to-br from-violet-950 via-violet-900 to-fuchsia-900 px-9 py-10 relative overflow-hidden">
        <div className="absolute -top-16 -right-16 w-72 h-72 rounded-full bg-fuchsia-500/20 blur-3xl pointer-events-none" />
        <div className="absolute bottom-0 left-0 w-48 h-48 rounded-full bg-violet-400/10 blur-3xl pointer-events-none" />

        <div className="relative z-10 flex flex-col h-full">
          <div className="mb-12"><img src="/logo/qorven-wordmark-white.svg" alt="Qorven" className="h-9" /></div>

          <div className="flex-1">
            <p className="text-xs font-semibold uppercase tracking-widest text-white/50 mb-5">Setup Steps</p>
            <div className="space-y-4">
              {[
                { n: 1, label: 'Account' },
                { n: 2, label: 'Workspace' },
                { n: 3, label: 'Provider' },
                { n: 4, label: 'Channels' },
                { n: 5, label: 'Done' },
              ].map(({ n, label }) => {
                const done   = accepted && visualStep > n;
                const active = accepted && visualStep === n;
                return (
                  <div key={n} className="flex items-center gap-3.5">
                    <div className={cn(
                      'flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold transition-all duration-200',
                      done   ? 'border-white/70 bg-white/20 text-white' :
                      active ? 'border-white      bg-white/15 text-white' :
                               'border-white/40   text-white/60'
                    )}>
                      {done ? <Check className="h-3 w-3" /> : n}
                    </div>
                    <span className={cn(
                      'text-sm font-medium transition-colors duration-200',
                      done   ? 'text-white/60 line-through decoration-white/30' :
                      active ? 'text-white' :
                               'text-white/75'
                    )}>{label}</span>
                  </div>
                );
              })}
            </div>
          </div>

          <div className="space-y-1">
            <p><a href="https://qorven.ai" target="_blank" rel="noopener noreferrer" className="text-sm font-medium text-white/60 hover:text-white/90 transition-colors">qorven.ai</a></p>
            {appVersion && <p className="text-xs text-white/50">Version {appVersion}</p>}
            <p className="text-xs text-white/40">&copy; 2026 Qorven AI</p>
          </div>
        </div>
      </div>

      {/* Right panel */}
      <div className="grow min-w-0 h-screen overflow-y-auto flex flex-col">
        <div className="flex-1 flex flex-col justify-center py-8 px-8">
        <div className="w-full max-w-[680px] mx-auto space-y-5">

          {globalError && (
            <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {globalError}
            </div>
          )}

          <div className="rounded-2xl border border-border bg-card/80 backdrop-blur-sm p-6 shadow-sm overflow-y-auto" style={{ minHeight: '420px', maxHeight: 'calc(100vh - 220px)' }}>
            <div className="space-y-5">
              {!accepted && (
                <CapabilitiesNotice onAccept={() => setAccepted(true)} version={appVersion} />
              )}
              {accepted && step === 1 && (
                <Step1Admin
                  displayName={displayName} setDisplayName={setDisplayName}
                  email={email} setEmail={setEmail}
                  username={username} setUsername={setUsername}
                  password={password} setPassword={setPassword}
                  showPw={showPw} setShowPw={setShowPw}
                  busy={creatingAdmin} adminCreated={adminCreated}
                  submit={handleCreateAdmin} skippable={!setupRequired}
                />
              )}
              {accepted && step === 2 && (
                <Step2Workspace
                  displayName={displayName}
                  primeName={primeName} setPrimeName={setPrimeName}
                  gradient={primeGradient} setGradient={setPrimeGradient}
                  callMe={callMe} setCallMe={setCallMe}
                  style={style} setStyle={setStyle}
                  language={language} setLanguage={setLanguage}
                />
              )}
              {accepted && step === 3 && (
                <Step4Provider
                  catalog={catalog}
                  selectedOption={selectedOption}
                  setSelectedOption={(v) => { setSelectedOption(v); setProviderStatus('idle'); setApiKey(''); setApiBase(''); setAwsAccessKey(''); setAwsSecretKey(''); }}
                  region={region} setRegion={setRegion}
                  apiKey={apiKey} setApiKey={setApiKey}
                  apiBase={apiBase} setApiBase={setApiBase}
                  awsAccessKey={awsAccessKey} setAwsAccessKey={setAwsAccessKey}
                  awsSecretKey={awsSecretKey} setAwsSecretKey={setAwsSecretKey}
                  status={providerStatus} error={providerError} sample={providerSample}
                  bedrockModels={bedrockModels}
                  primary={primaryModel} setPrimary={setPrimaryModel}
                  fast={fastModel} setFast={setFastModel}
                  coding={codingModel} setCoding={setCodingModel}
                  setup={setupProvider}
                  addedProviders={addedProviders}
                />
              )}
              {accepted && step === 4 && (
                <Step7Channels
                  primeName={primeName}
                  telegram={telegramToken} setTelegram={setTelegramToken}
                  connected={connectedChannels} busy={channelBusy} error={channelError}
                  connect={connectChannel}
                  onSkip={() => setStep(5)}
                />
              )}
              {accepted && step === 5 && (
                <Step9Summary
                  workspaceName="My Workspace"
                  username={username || 'admin'}
                  primeName={primeName}
                  region={region}
                  selectedProvider={selectedOption}
                  primaryModel={primaryModel}
                  connectedChannels={connectedChannels}
                  onDone={finalise}
                />
              )}

            </div>
          </div>

          {(accepted && step !== 5) && (
            <div className="flex items-center justify-between">
              <button
                onClick={goBack}
                disabled={step === 1}
                className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3 py-2 text-xs text-muted-foreground hover:bg-accent disabled:opacity-40 cursor-pointer">
                <ArrowLeft className="h-3.5 w-3.5" /> Back
              </button>
              <button
                onClick={step === 1 ? handleCreateAdmin : goNext}
                disabled={step === 1 ? creatingAdmin : isNextDisabled()}
                className="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-40 cursor-pointer">
                {step === 1 && creatingAdmin
                  ? <><QorvenSpinner className="h-3.5 w-3.5" /> Creating…</>
                  : step === 1 && !adminCreated
                  ? <>Create Account <ArrowRight className="h-3.5 w-3.5" /></>
                  : <>Continue <ArrowRight className="h-3.5 w-3.5" /></>
                }
              </button>
            </div>
          )}
        </div>
        </div>
      </div>
    </div>
  );
}
