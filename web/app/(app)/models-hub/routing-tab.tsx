'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useState, useCallback } from 'react';
import { BarChart3, Loader2, Check, Zap, Trophy, ExternalLink, ChevronDown, ChevronUp } from 'lucide-react';
import { cn } from '@/lib/utils';
import { routing, routingTyped, type RoutingDecision, type RankedModel, type ModelRankingsResponse } from '@/lib/api';
import { toast } from 'sonner';

type SelModel  = { model_id: string; provider_id: string; is_default?: boolean };
type Category  = string;
type Assignments = Record<string, string[]>;

export function RoutingTab({ selectedModels }: { selectedModels: SelModel[] }) {
  const [categories, setCategories] = useState<Category[]>([]);
  const [assignments, setAssignments] = useState<Assignments>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState<string | null>(null);
  const [suggesting, setSuggesting] = useState(false);

  const allModelIds = [...new Set(selectedModels.map(m => m.model_id))].sort();

  const load = useCallback(() => {
    setLoading(true);
    Promise.all([
      routing.categories().catch(() => [] as any[]),
      routing.assignments().catch(() => ({} as Assignments)),
    ]).then(([cats, asgn]) => {
      setCategories(Array.isArray(cats) ? cats : []);
      setAssignments(asgn ?? {});
      setLoading(false);
    });
  }, []);

  useEffect(() => { load(); }, [load]);

  const assign = async (category: string, modelId: string) => {
    setSaving(category);
    try {
      const current = assignments[category] ?? [];
      await Promise.all(current.map(m => routing.unassign(category, m)));
      if (modelId) await routing.assign(category, modelId);
      setAssignments(prev => ({ ...prev, [category]: modelId ? [modelId] : [] }));
    } catch { toast.error('Failed to update routing'); }
    finally { setSaving(null); }
  };

  const autoSuggest = async () => {
    setSuggesting(true);
    try {
      const suggestions = await routing.suggestions(allModelIds.length ? allModelIds : undefined);
      const ops: Promise<void>[] = [];
      for (const [cat, modelId] of Object.entries(suggestions)) {
        const current = (assignments[cat] ?? [])[0];
        if (!current && modelId) {
          ops.push(routing.assign(cat, modelId));
        }
      }
      await Promise.all(ops);
      await load();
      toast.success('Suggestions applied to unassigned categories');
    } catch { toast.error('Auto-suggest failed'); }
    finally { setSuggesting(false); }
  };

  if (loading) return (
    <div className="flex items-center gap-2 py-12 justify-center text-muted-foreground">
      <Loader2 className="h-4 w-4 animate-spin" /> Loading routing config…
    </div>
  );

  if (categories.length === 0) return (
    <div className="py-12 text-center text-muted-foreground">
      <BarChart3 className="h-8 w-8 mx-auto mb-2 opacity-30" />
      <p className="text-sm">No routing categories found.</p>
      <p className="text-xs mt-1">Categories are discovered from SmartRouter configuration.</p>
    </div>
  );

  return (
    <div className="space-y-4">
      <ModelRankingsPanel onSelectModel={(id) => {
        // Pre-assign the selected model to any unassigned categories that
        // best match its strengths (coding → coding category, etc.).
        // For now just scroll to the category table as a UX hint.
      }} />

      <div className="flex items-start justify-between gap-3">
        <p className="text-sm text-muted-foreground pb-1">
          Assign a model to each work category. The SmartRouter selects the best model based on query type.
          Only your selected models appear in the dropdown.
        </p>
        <button
          onClick={autoSuggest}
          disabled={suggesting}
          className="shrink-0 flex items-center gap-1.5 rounded-lg border border-primary/40 bg-primary/5 px-3 py-1.5 text-xs font-medium text-primary hover:bg-primary/10 disabled:opacity-50 transition-colors"
          title="Auto-fill unassigned categories with best-fit models from the catalog"
        >
          {suggesting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Zap className="h-3.5 w-3.5" />}
          Auto-suggest
        </button>
      </div>

      <div className="rounded-xl border border-border overflow-hidden mb-4">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/40 text-xs text-muted-foreground">
              <th className="text-left px-4 py-2.5 font-medium">Category</th>
              <th className="text-left px-4 py-2.5 font-medium">Assigned Model</th>
            </tr>
          </thead>
          <tbody>
            {categories.map(cat => {
              const current = (assignments[cat] ?? [])[0] ?? '';
              const isSaving = saving === cat;
              return (
                <tr key={cat} className="border-b border-border/40 last:border-0 hover:bg-accent/20 transition-colors">
                  <td className="px-4 py-2.5">
                    <p className="font-medium capitalize">{cat.replace(/_/g, ' ')}</p>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <select
                        value={current}
                        onChange={e => assign(cat, e.target.value)}
                        disabled={isSaving}
                        className="qr-select flex-1 max-w-xs text-xs"
                      >
                        <option value="">— unassigned —</option>
                        {allModelIds.map(id => (
                          <option key={id} value={id}>{id}</option>
                        ))}
                      </select>
                      {isSaving && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
                      {current && !isSaving && <Check className="h-3.5 w-3.5 text-emerald-400 shrink-0" />}
                    </div>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <RoutingDecisionsPanel categories={categories} modelIds={allModelIds} />
    </div>
  );
}

// ── Model Rankings Panel ──────────────────────────────────────────────────────

function ModelRankingsPanel({ onSelectModel }: { onSelectModel: (id: string) => void }) {
  const [data, setData] = useState<ModelRankingsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    routing.modelRankings()
      .then(setData)
      .catch(() => setData(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return (
    <div className="flex items-center gap-2 py-3 text-xs text-muted-foreground">
      <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading model rankings…
    </div>
  );

  if (!data?.configured) {
    return (
      <div className="rounded-xl border border-border bg-muted/20 px-4 py-3 flex items-start gap-3">
        <Trophy className="h-4 w-4 text-muted-foreground/50 mt-0.5 shrink-0" />
        <div className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">Model rankings unavailable.</span>{' '}
          Add your Artificial Analysis API key in{' '}
          <a href="/models-hub/integrations" className="text-primary hover:underline">
            Models → Integrations
          </a>{' '}
          to show live benchmark scores and rankings that improve auto-routing.{' '}
          <a
            href="https://artificialanalysis.ai"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-0.5 text-muted-foreground hover:text-primary"
          >
            Get API key <ExternalLink className="h-2.5 w-2.5" />
          </a>
        </div>
      </div>
    );
  }

  if (!data.models.length) {
    return (
      <div className="rounded-xl border border-border bg-muted/20 px-4 py-3 text-xs text-muted-foreground">
        Rankings are being fetched — check back in a moment.
      </div>
    );
  }

  const visible = expanded ? data.models : data.models.slice(0, 10);

  return (
    <div className="rounded-xl border border-border overflow-hidden">
      <header className="flex items-center gap-2 border-b border-border bg-muted/40 px-4 py-2">
        <Trophy className="h-3.5 w-3.5 text-amber-400" />
        <h3 className="text-xs font-semibold uppercase tracking-wider">Model Rankings</h3>
        <a
          href="https://artificialanalysis.ai"
          target="_blank"
          rel="noopener noreferrer"
          className="ml-1 inline-flex items-center gap-0.5 text-2xs text-muted-foreground hover:text-primary transition-colors"
        >
          Powered by Artificial Analysis <ExternalLink className="h-2.5 w-2.5" />
        </a>
        {data.fetched_at && (
          <span className="ml-auto font-mono text-2xs text-muted-foreground/60">
            {new Date(data.fetched_at).toLocaleDateString()}
          </span>
        )}
      </header>

      <table className="w-full text-xs">
        <thead>
          <tr className="border-b border-border/60 text-2xs uppercase tracking-wider text-muted-foreground">
            <th className="px-3 py-1.5 text-right font-medium w-10">#</th>
            <th className="px-3 py-1.5 text-left font-medium">Model</th>
            <th className="px-3 py-1.5 text-right font-medium" title="Intelligence Index">Intel</th>
            <th className="px-3 py-1.5 text-right font-medium" title="Coding Index">Code</th>
            <th className="px-3 py-1.5 text-right font-medium" title="Median output tokens/sec">Speed</th>
            <th className="px-3 py-1.5 text-right font-medium" title="Input price per 1M tokens">$/1M in</th>
            <th className="px-3 py-1.5 text-right font-medium" title="Output price per 1M tokens">$/1M out</th>
          </tr>
        </thead>
        <tbody>
          {visible.map((m) => (
            <tr
              key={m.id}
              className="border-b border-border/30 last:border-0 hover:bg-accent/20 transition-colors cursor-pointer"
              onClick={() => onSelectModel(m.id)}
              title={`Click to use ${m.name} in routing`}
            >
              <td className="px-3 py-1.5 text-right font-mono text-muted-foreground">{m.rank}</td>
              <td className="px-3 py-1.5">
                <p className="font-medium truncate max-w-[180px]">{m.name}</p>
                <p className="text-2xs text-muted-foreground/70 truncate">{m.organization}</p>
              </td>
              <td className="px-3 py-1.5 text-right font-mono">
                {m.intelligence_index > 0 ? m.intelligence_index.toFixed(1) : '—'}
              </td>
              <td className="px-3 py-1.5 text-right font-mono">
                {m.coding_index > 0 ? m.coding_index.toFixed(1) : '—'}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-muted-foreground">
                {m.speed_tokens_per_sec > 0 ? `${Math.round(m.speed_tokens_per_sec)}` : '—'}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-emerald-400/80">
                {m.input_price_per_m > 0 ? `$${m.input_price_per_m.toFixed(2)}` : '—'}
              </td>
              <td className="px-3 py-1.5 text-right font-mono text-emerald-400/80">
                {m.output_price_per_m > 0 ? `$${m.output_price_per_m.toFixed(2)}` : '—'}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {data.models.length > 10 && (
        <button
          onClick={() => setExpanded(e => !e)}
          className="w-full flex items-center justify-center gap-1 py-2 text-2xs text-muted-foreground hover:text-foreground hover:bg-accent/20 transition-colors border-t border-border/40"
        >
          {expanded
            ? <><ChevronUp className="h-3 w-3" /> Show less</>
            : <><ChevronDown className="h-3 w-3" /> Show all {data.models.length} models</>}
        </button>
      )}
    </div>
  );
}

function RoutingDecisionsPanel({ categories, modelIds }: { categories: string[]; modelIds: string[] }) {
  const [rows, setRows] = useState<RoutingDecision[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setLoading(true);
    routingTyped.decisions().then(setRows).catch(() => setRows([])).finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="rounded-xl border border-border overflow-hidden">
      <header className="flex items-center gap-2 border-b border-border bg-muted/40 px-4 py-2">
        <BarChart3 className="h-3.5 w-3.5 text-muted-foreground" />
        <h3 className="text-xs font-semibold uppercase tracking-wider">Recent decisions</h3>
        <span className="ml-auto font-mono text-2xs text-muted-foreground">{rows.length}</span>
      </header>
      {loading ? (
        <div className="flex items-center justify-center gap-2 py-6 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading…
        </div>
      ) : rows.length === 0 ? (
        <p className="px-4 py-6 text-center text-2xs text-muted-foreground">
          No routing decisions yet. They accumulate as agents make LLM calls.
        </p>
      ) : (
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-border/60 text-2xs uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-1.5 text-left font-medium">Query</th>
              <th className="px-4 py-1.5 text-left font-medium">Category</th>
              <th className="px-4 py-1.5 text-left font-medium">Model</th>
              <th className="px-4 py-1.5 text-left font-medium">Confidence</th>
              <th className="px-4 py-1.5 text-right font-medium">Action</th>
            </tr>
          </thead>
          <tbody>
            {rows.slice(0, 20).map((d) => (
              <RoutingDecisionRow
                key={d.id}
                decision={d}
                categories={categories}
                modelIds={modelIds}
                onCorrected={load}
              />
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function RoutingDecisionRow({ decision, categories, modelIds, onCorrected }: {
  decision: RoutingDecision;
  categories: string[];
  modelIds: string[];
  onCorrected: () => void;
}) {
  const [editing, setEditing] = useState(false);
  const [model, setModel] = useState(decision.override_model || decision.model);
  const [category, setCategory] = useState(decision.override_category || decision.category);
  const [busy, setBusy] = useState(false);

  const wasCorrectedAlready = decision.was_correct === false;

  const submit = async () => {
    setBusy(true);
    try {
      await routingTyped.correct(decision.id, model, category);
      toast.success('Correction recorded');
      setEditing(false);
      onCorrected();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Correct failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <tr className={cn('border-b border-border/30 last:border-0 align-top', wasCorrectedAlready && 'bg-amber-400/5')}>
      <td className="px-4 py-2 max-w-xs">
        <p className="truncate text-foreground" title={decision.query_preview}>{decision.query_preview}</p>
        <p className="font-mono text-2xs text-muted-foreground/70">{new Date(decision.created_at).toLocaleString()}</p>
      </td>
      <td className="px-4 py-2">
        {editing ? (
          <select value={category} onChange={(e) => setCategory(e.target.value)}
            className="qr-input font-mono text-2xs h-6 py-0.5 px-1.5">
            {categories.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        ) : (
          <span className="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2xs capitalize text-muted-foreground">
            {decision.override_category ?? decision.category}
          </span>
        )}
      </td>
      <td className="px-4 py-2 font-mono text-2xs">
        {editing ? (
          <select value={model} onChange={(e) => setModel(e.target.value)}
            className="qr-select text-xs h-6 py-0.5 px-1.5">
            {modelIds.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
        ) : (
          <span className={decision.override_model ? 'text-amber-400' : ''} title={decision.override_model ? 'Corrected' : undefined}>
            {decision.override_model ?? decision.model}
          </span>
        )}
      </td>
      <td className="px-4 py-2 font-mono text-2xs">
        {decision.confidence != null ? `${(decision.confidence * 100).toFixed(0)}%` : '—'}
      </td>
      <td className="px-4 py-2 text-right">
        {editing ? (
          <span className="inline-flex items-center gap-1">
            <button onClick={submit} disabled={busy}
              className="rounded-sm border border-primary/40 bg-primary/10 px-2 py-0.5 font-mono text-2xs text-primary hover:bg-primary/20 disabled:opacity-50">
              {busy ? <Loader2 className="h-3 w-3 animate-spin" /> : 'save'}
            </button>
            <button onClick={() => setEditing(false)} className="font-mono text-2xs text-muted-foreground hover:text-foreground">
              cancel
            </button>
          </span>
        ) : wasCorrectedAlready ? (
          <span className="font-mono text-2xs text-amber-400">corrected</span>
        ) : (
          <button onClick={() => setEditing(true)}
            className="rounded-sm border border-border bg-card px-2 py-0.5 font-mono text-2xs text-muted-foreground hover:bg-accent">
            mark incorrect
          </button>
        )}
      </td>
    </tr>
  );
}
