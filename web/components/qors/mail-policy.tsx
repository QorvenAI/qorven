'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState, useEffect } from 'react';
import { Save, Sparkles, Plus, Trash2, ChevronDown, ChevronRight } from 'lucide-react';
import { agents } from '@/lib/api-agents';
import { listInboundRules, deleteInboundRule, type InboundRule } from '@/lib/api-inbound';
import { SearchableSelect } from '@/components/searchable-select';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import type { Soul } from '@/types';

const MODE_LABELS: Record<string, string> = {
  fully_autonomous: 'Auto-reply',
  draft_and_approve: 'Draft & approve',
  draft_only: 'Draft only',
  context_only: 'Log only',
  drop: 'Drop',
};

const MATCH_LABELS: Record<string, string> = {
  contact: 'Contact',
  domain: 'Domain',
  keyword: 'Keyword',
  default: 'Default (catch-all)',
};

function RuleBadge({ rule }: { rule: InboundRule }) {
  return (
    <span
      className={cn(
        'rounded-full px-2 py-0.5 text-[10px] font-medium',
        rule.status === 'pending_confirmation'
          ? 'bg-amber-500/10 text-amber-600'
          : 'bg-emerald-500/10 text-emerald-600'
      )}
    >
      {rule.status === 'pending_confirmation' ? 'Pending' : 'Active'}
    </span>
  );
}

export function MailPolicy({ agentId }: { agentId: string }) {
  const [soul, setSoul] = useState<Soul | null>(null);
  const [policy, setPolicy] = useState('');
  const [rules, setRules] = useState<InboundRule[]>([]);
  const [saving, setSaving] = useState(false);
  const [briefingOpen, setBriefingOpen] = useState(false);

  useEffect(() => {
    agents.get(agentId).then((s) => {
      setSoul(s);
      setPolicy((s as any).mail_policy ?? '');
    }).catch(() => {});
    listInboundRules(agentId).then(setRules).catch(() => {});
  }, [agentId]);

  const savePolicy = async () => {
    setSaving(true);
    try {
      await agents.update(agentId, { mail_policy: policy } as any);
      toast.success('Mail policy saved');
    } catch {
      toast.error('Failed to save policy');
    } finally {
      setSaving(false);
    }
  };

  const removeRule = async (ruleId: string) => {
    await deleteInboundRule(agentId, ruleId).catch(() => {});
    setRules((r) => r.filter((x) => x.id !== ruleId));
  };

  const displayName = soul?.display_name ?? 'this agent';

  return (
    <div className="mx-auto max-w-2xl space-y-8 p-6">
      {/* Natural-language policy */}
      <section>
        <h3 className="mb-1 text-sm font-semibold">Mail Policy</h3>
        <p className="mb-3 text-xs text-muted-foreground">
          Plain English. {displayName} interprets this at runtime for each incoming email.
        </p>
        <textarea
          value={policy}
          onChange={(e) => setPolicy(e.target.value)}
          rows={6}
          placeholder={`e.g. "Auto-reply to my team at @company.com. Draft replies for new contacts and hold for my approval. Drop anything that looks like spam or has 'unsubscribe' in the footer."`}
          className="block w-full resize-none rounded-xl border border-border bg-background px-4 py-3 text-sm leading-relaxed focus:outline-none focus:ring-2 focus:ring-primary/30"
        />

        {policy.trim() && (
          <div className="mt-3 rounded-xl border border-primary/20 bg-primary/5 p-4">
            <div className="mb-2 flex items-center gap-1.5 text-xs font-semibold text-primary">
              <Sparkles className="h-3.5 w-3.5" />
              How I understand this policy
            </div>
            <p className="text-xs text-muted-foreground leading-relaxed">
              Save the policy to see {displayName}&apos;s interpretation here. The agent will explain
              what it will auto-reply to, what it will hold for approval, and what it will drop.
            </p>
          </div>
        )}

        <button
          onClick={savePolicy}
          disabled={saving}
          className="mt-3 flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          <Save className="h-3.5 w-3.5" />
          {saving ? 'Saving…' : 'Save Policy'}
        </button>
      </section>

      {/* Derived / chat-set rules */}
      <section>
        <div className="mb-3 flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold">Routing Rules</h3>
            <p className="text-xs text-muted-foreground">
              Set by chatting with {displayName} or Prime. Rules override the policy above when there&apos;s a conflict.
            </p>
          </div>
        </div>

        {rules.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No rules yet. Tell {displayName} "always auto-reply to sarah@acme.com" to create one.
          </p>
        ) : (
          <div className="overflow-hidden rounded-xl border border-border">
            <table className="w-full text-xs">
              <thead className="bg-muted/30">
                <tr>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Match</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Value</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Action</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Status</th>
                  <th className="px-3 py-2" />
                </tr>
              </thead>
              <tbody>
                {rules.map((r) => (
                  <tr key={r.id} className="border-t border-border">
                    <td className="px-3 py-2 text-muted-foreground">{MATCH_LABELS[r.match_type] ?? r.match_type}</td>
                    <td className="px-3 py-2 font-mono max-w-[120px] truncate">{r.match_value || '—'}</td>
                    <td className="px-3 py-2">{MODE_LABELS[r.mode] ?? r.mode}</td>
                    <td className="px-3 py-2"><RuleBadge rule={r} /></td>
                    <td className="px-3 py-2">
                      <button
                        onClick={() => removeRule(r.id)}
                        className="text-muted-foreground hover:text-destructive transition-colors"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Briefing */}
      <section>
        <button
          onClick={() => setBriefingOpen((o) => !o)}
          className="flex w-full items-center justify-between rounded-xl border border-border px-4 py-3 text-sm font-medium hover:bg-accent/50"
        >
          Daily Briefing
          {briefingOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </button>
        {briefingOpen && (
          <BriefingSettings agentId={agentId} />
        )}
      </section>
    </div>
  );
}

function BriefingSettings({ agentId }: { agentId: string }) {
  const [enabled, setEnabled] = useState(false);
  const [time, setTime] = useState('08:00');
  const [tz, setTz] = useState('Asia/Shanghai');
  const [prompt, setPrompt] = useState('Summarise emails I missed overnight, highlight anything needing my response.');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    import('@/lib/api-inbound').then(({ getInboundConfig }) => {
      getInboundConfig(agentId).then((cfg) => {
        setEnabled(cfg.briefing_enabled);
        if (cfg.briefing_time) setTime(cfg.briefing_time);
        if (cfg.briefing_timezone) setTz(cfg.briefing_timezone);
      }).catch(() => {});
    });
  }, [agentId]);

  const save = async () => {
    setSaving(true);
    const { putInboundConfig } = await import('@/lib/api-inbound');
    try {
      await putInboundConfig(agentId, {
        briefing_enabled: enabled,
        briefing_time: time,
        briefing_timezone: tz,
      });
      toast.success('Briefing settings saved');
    } catch {
      toast.error('Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mt-2 rounded-xl border border-border bg-card p-4 space-y-3">
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="rounded"
        />
        Enable morning briefing digest
      </label>
      {enabled && (
        <>
          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs text-muted-foreground">Time (HH:MM)</span>
              <input
                type="text"
                value={time}
                onChange={(e) => setTime(e.target.value)}
                placeholder="08:00"
                className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
              />
            </label>
            <label className="block">
              <span className="text-xs text-muted-foreground">Timezone</span>
              <input
                type="text"
                value={tz}
                onChange={(e) => setTz(e.target.value)}
                placeholder="Asia/Shanghai"
                className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
              />
            </label>
          </div>
          <label className="block">
            <span className="text-xs text-muted-foreground">What should be in my briefing?</span>
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={3}
              className="mt-1 block w-full resize-none rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
            />
          </label>
        </>
      )}
      <button
        onClick={save}
        disabled={saving}
        className="flex items-center gap-2 rounded-lg bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        <Save className="h-3 w-3" />
        {saving ? 'Saving…' : 'Save Briefing'}
      </button>
    </div>
  );
}
