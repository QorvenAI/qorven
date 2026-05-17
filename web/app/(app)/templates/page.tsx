'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { useRouter } from 'next/navigation';
import {
  Sparkles, LayoutTemplate, Zap, Check, Loader2, ArrowRight,
  Plus, Users, BarChart3, Globe, Bot, Search, X, ExternalLink,
  CheckCircle2, ChevronRight,
} from 'lucide-react';
import { workspaces } from '@/lib/api';
import { ErrorBoundary } from '@/components/error-boundary';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

type Template = {
  id: string; name: string; description: string; icon: string;
  category: string; agents: any[]; dashboard: any;
  skills: string[]; connectors: string[];
};

type InstalledWorkspace = {
  template_id: string; name: string; agent_count?: number;
};

const CATEGORY_LABELS: Record<string, string> = {
  business: 'Business', marketing: 'Marketing', knowledge: 'Knowledge',
  finance: 'Finance', engineering: 'Engineering', professional: 'Professional',
  analytics: 'Analytics', education: 'Education',
};

export default function TemplatesPage() {
  const router = useRouter();
  const [templates, setTemplates] = useState<Template[]>([]);
  const [installed, setInstalled] = useState<InstalledWorkspace[]>([]);
  const [loading, setLoading] = useState(true);
  const [installing, setInstalling] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [activeCategory, setActiveCategory] = useState('all');
  const [showBuilder, setShowBuilder] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [tmpl, inst] = await Promise.all([
        workspaces.listTemplates().catch(() => []),
        workspaces.listInstalled().catch(() => []),
      ]);
      setTemplates(Array.isArray(tmpl) ? tmpl : []);
      setInstalled(Array.isArray(inst) ? inst : []);
    } finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const install = async (templateId: string, name: string) => {
    setInstalling(templateId);
    try {
      const result = await workspaces.install(templateId) as any;
      toast.success(`"${name}" installed — ${result?.agent_count ?? 0} agents created`);
      load();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Installation failed');
    } finally { setInstalling(null); }
  };

  const installedIds = new Set(installed.map(i => i.template_id));
  const categories = ['all', ...Array.from(new Set(templates.map(t => t.category).filter(Boolean)))];

  const filtered = templates.filter(t => {
    const matchesSearch = !search || t.name.toLowerCase().includes(search.toLowerCase()) || t.description.toLowerCase().includes(search.toLowerCase());
    const matchesCategory = activeCategory === 'all' || t.category === activeCategory;
    return matchesSearch && matchesCategory;
  });

  return (
    <ErrorBoundary>
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold flex items-center gap-2">
              <LayoutTemplate className="h-6 w-6" /> Workspaces
            </h1>
            <p className="text-sm text-muted-foreground mt-0.5">
              One-click AI workspaces — install a pre-built team or describe what you need
            </p>
          </div>
          <div className="flex items-center gap-2">
            <button onClick={() => router.push('/templates/new')}
              className="flex items-center gap-2 rounded-xl border border-border px-4 py-2.5 text-sm font-medium hover:bg-accent cursor-pointer transition-colors">
              <Plus className="h-4 w-4" /> Custom Builder
            </button>
            <button onClick={() => setShowBuilder(true)}
              className="flex items-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-primary-foreground hover:bg-primary/90 cursor-pointer shadow-lg shadow-primary/20 transition-all hover:scale-[1.02]">
              <Sparkles className="h-4 w-4" /> Build with AI
            </button>
          </div>
        </div>

        {/* AI Builder */}
        {showBuilder && (
          <AIBuilder
            onClose={() => setShowBuilder(false)}
            onInstalled={(id) => { setShowBuilder(false); load(); router.push(`/dashboard/${id}`); }}
          />
        )}

        {/* Installed workspaces */}
        {installed.length > 0 && (
          <div>
            <h2 className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-3">Your Workspaces</h2>
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
              {installed.map(w => (
                <button key={w.template_id} onClick={() => router.push(`/dashboard/${w.template_id}`)}
                  className="flex items-center gap-3 rounded-xl border border-primary/30 bg-primary/5 px-4 py-3 text-left hover:bg-primary/10 cursor-pointer transition-colors group">
                  <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 shrink-0 text-xl">
                    {templates.find(t => t.id === w.template_id)?.icon ?? '🚀'}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold truncate">{w.name}</p>
                    <p className="text-xs text-muted-foreground">{w.agent_count ?? '?'} agents · Active</p>
                  </div>
                  <ChevronRight className="h-4 w-4 text-muted-foreground group-hover:text-foreground shrink-0" />
                </button>
              ))}
            </div>
          </div>
        )}

        {/* Search + Filter */}
        <div className="flex items-center gap-3 flex-wrap">
          <div className="relative flex-1 min-w-48 max-w-sm">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search workspaces…"
              className="qr-input pl-9" />
            {search && (
              <button onClick={() => setSearch('')} className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground cursor-pointer">
                <X className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
          <div className="flex gap-1.5 flex-wrap">
            {categories.map(cat => (
              <button key={cat} onClick={() => setActiveCategory(cat)}
                className={cn('rounded-lg px-3 py-1.5 text-xs font-medium transition-colors cursor-pointer capitalize',
                  activeCategory === cat ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground hover:text-foreground hover:bg-accent')}>
                {cat === 'all' ? 'All' : (CATEGORY_LABELS[cat] ?? cat)}
              </button>
            ))}
          </div>
        </div>

        {/* Grid */}
        {loading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="rounded-xl border border-border bg-card p-5 space-y-3 animate-pulse">
                <div className="flex gap-3"><div className="h-12 w-12 rounded-xl bg-muted shrink-0" /><div className="flex-1 space-y-2"><div className="h-4 w-28 rounded bg-muted" /><div className="h-3 w-16 rounded bg-muted" /></div></div>
                <div className="h-3 w-full rounded bg-muted" /><div className="h-3 w-4/5 rounded bg-muted" />
                <div className="h-9 w-full rounded bg-muted" />
              </div>
            ))}
          </div>
        ) : filtered.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border py-16 text-center">
            <LayoutTemplate className="h-10 w-10 mx-auto mb-3 text-muted-foreground/30" />
            <p className="text-sm text-muted-foreground">{search ? `No workspaces match "${search}"` : 'No workspaces available'}</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {filtered.map(t => (
              <TemplateCard key={t.id} template={t}
                isInstalled={installedIds.has(t.id)}
                isInstalling={installing === t.id}
                onInstall={() => install(t.id, t.name)}
                onOpen={() => router.push(`/dashboard/${t.id}`)}
              />
            ))}
          </div>
        )}

        <div className="flex gap-6 pt-2 border-t border-border text-xs text-muted-foreground">
          <span>{templates.length} workspaces</span>
          <span>{installed.length} installed</span>
          <span>{categories.length - 1} categories</span>
        </div>
      </div>
    </ErrorBoundary>
  );
}

function TemplateCard({ template, isInstalled, isInstalling, onInstall, onOpen }: {
  template: Template; isInstalled: boolean; isInstalling: boolean;
  onInstall: () => void; onOpen: () => void;
}) {
  const agentCount = template.agents?.length ?? 0;
  const blockCount = template.dashboard?.blocks?.length ?? 0;
  return (
    <div className={cn('group relative flex flex-col rounded-xl border bg-card p-5 transition-all hover:shadow-md',
      isInstalled ? 'border-primary/30 bg-primary/[0.02]' : 'border-border hover:border-primary/20')}>
      {isInstalled && (
        <span className="absolute top-3 right-3 flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs font-medium text-emerald-500">
          <CheckCircle2 className="h-3 w-3" /> Installed
        </span>
      )}
      <div className="flex items-start gap-3 mb-3">
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-muted text-2xl">{template.icon}</div>
        <div className="min-w-0 flex-1 pr-12">
          <h3 className="text-sm font-semibold leading-tight">{template.name}</h3>
          <span className="text-xs text-muted-foreground capitalize">{CATEGORY_LABELS[template.category] ?? template.category}</span>
        </div>
      </div>
      <p className="text-xs text-muted-foreground leading-relaxed mb-4 flex-1">{template.description}</p>
      <div className="flex flex-wrap gap-1.5 mb-4">
        <span className="flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">
          <Users className="h-2.5 w-2.5" /> {agentCount} agents
        </span>
        <span className="flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">
          <BarChart3 className="h-2.5 w-2.5" /> {blockCount} widgets
        </span>
        {template.connectors?.slice(0, 2).map(c => (
          <span key={c} className="rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">{c}</span>
        ))}
        {(template.connectors?.length ?? 0) > 2 && (
          <span className="rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">+{template.connectors.length - 2}</span>
        )}
      </div>
      {isInstalled ? (
        <button onClick={onOpen} className="flex items-center justify-center gap-1.5 w-full rounded-lg bg-primary/10 text-primary px-4 py-2 text-sm font-medium hover:bg-primary/20 cursor-pointer transition-colors">
          Open Workspace <ArrowRight className="h-3.5 w-3.5" />
        </button>
      ) : (
        <button onClick={onInstall} disabled={isInstalling}
          className="flex items-center justify-center gap-1.5 w-full rounded-lg border border-border px-4 py-2 text-sm font-medium hover:bg-accent hover:border-primary/30 disabled:opacity-50 cursor-pointer transition-colors">
          {isInstalling ? <><Loader2 className="h-3.5 w-3.5 animate-spin" /> Installing…</> : <><Zap className="h-3.5 w-3.5" /> Install Workspace</>}
        </button>
      )}
    </div>
  );
}

function AIBuilder({ onClose, onInstalled }: { onClose: () => void; onInstalled: (id: string) => void }) {
  const [description, setDescription] = useState('');
  const [building, setBuilding] = useState(false);
  const [result, setResult] = useState<any>(null);

  const EXAMPLES = [
    'Build me a CRM for a SaaS startup',
    'I need a content marketing team',
    'Set up a freelance project manager',
    'Create an e-commerce analytics workspace',
    'I want a devops monitoring setup',
    'Build an HR and recruiting workspace',
  ];

  const build = async () => {
    if (!description.trim()) return;
    setBuilding(true);
    setResult(null);
    try {
      const res = await workspaces.selfBuild(description) as any;
      setResult(res);
      toast.success(`"${res.name}" workspace ready!`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Build failed');
    } finally { setBuilding(false); }
  };

  return (
    <div className="rounded-2xl border border-primary/30 bg-gradient-to-br from-primary/5 to-background overflow-hidden shadow-lg">
      <div className="flex items-center justify-between px-6 py-4 border-b border-border">
        <div className="flex items-center gap-2">
          <div className="h-8 w-8 rounded-lg bg-primary flex items-center justify-center">
            <Sparkles className="h-4 w-4 text-primary-foreground" />
          </div>
          <div>
            <p className="text-sm font-semibold">AI Workspace Builder</p>
            <p className="text-xs text-muted-foreground">Describe what you need — we'll build it</p>
          </div>
        </div>
        <button onClick={onClose} className="text-muted-foreground hover:text-foreground cursor-pointer"><X className="h-4 w-4" /></button>
      </div>
      <div className="p-6 space-y-4">
        {!result ? (
          <>
            <div className="rounded-xl border border-border bg-background overflow-hidden focus-within:border-primary transition-colors">
              <textarea value={description} onChange={e => setDescription(e.target.value)}
                placeholder="Describe your workspace… e.g. 'Build me a CRM for a B2B SaaS startup with lead tracking and outreach automation'"
                rows={3} autoFocus
                onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) build(); }}
                className="w-full px-4 py-3 text-sm bg-transparent resize-none outline-none placeholder:text-muted-foreground/50" />
              <div className="flex items-center justify-between px-4 py-2 border-t border-border bg-muted/20">
                <span className="text-xs text-muted-foreground">⌘+Enter to build</span>
                <button onClick={build} disabled={building || !description.trim()}
                  className="flex items-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-1.5 text-xs font-semibold hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
                  {building ? <><Loader2 className="h-3 w-3 animate-spin" /> Building…</> : <><Zap className="h-3 w-3" /> Build</>}
                </button>
              </div>
            </div>
            <div>
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">Try an example</p>
              <div className="flex flex-wrap gap-2">
                {EXAMPLES.map(ex => (
                  <button key={ex} onClick={() => setDescription(ex)}
                    className="rounded-lg border border-border px-3 py-1.5 text-xs text-muted-foreground hover:text-foreground hover:border-primary/30 hover:bg-accent cursor-pointer transition-colors">
                    {ex}
                  </button>
                ))}
              </div>
            </div>
          </>
        ) : (
          <div className="space-y-4">
            <div className="flex items-start gap-3">
              <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-2xl">{result.icon ?? '🚀'}</div>
              <div>
                <p className="text-base font-semibold">{result.name}</p>
                <p className="text-sm text-muted-foreground">{result.description}</p>
              </div>
            </div>
            <div className="grid grid-cols-3 gap-3">
              {[
                { label: 'Agents', value: result.agent_count ?? 0, icon: Bot },
                { label: 'Template', value: result.template_id?.split('-')[0], icon: LayoutTemplate },
                { label: 'Status', value: 'Ready ✓', icon: CheckCircle2 },
              ].map(({ label, value, icon: Icon }) => (
                <div key={label} className="rounded-xl border border-border bg-card px-4 py-3 text-center">
                  <Icon className="h-4 w-4 mx-auto mb-1 text-primary" />
                  <p className="text-sm font-semibold truncate">{value}</p>
                  <p className="text-xs text-muted-foreground">{label}</p>
                </div>
              ))}
            </div>
            <div className="flex gap-2">
              <button onClick={() => onInstalled(result.template_id)}
                className="flex-1 flex items-center justify-center gap-1.5 rounded-lg bg-primary text-primary-foreground px-4 py-2.5 text-sm font-medium hover:bg-primary/90 cursor-pointer">
                <ExternalLink className="h-4 w-4" /> Open Workspace
              </button>
              <button onClick={() => { setResult(null); setDescription(''); }}
                className="rounded-lg border border-border px-4 py-2.5 text-sm hover:bg-accent cursor-pointer">
                Build Another
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
