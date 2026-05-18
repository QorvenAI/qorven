'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect } from 'react';
import { Plus, Trash2, Save } from 'lucide-react';
import {
  type ActionMode,
  type InboundConfig,
  type InboundRule,
  getInboundConfig,
  putInboundConfig,
  listInboundRules,
  createInboundRule,
  deleteInboundRule,
} from '@/lib/api-inbound';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { SearchableSelect } from '@/components/searchable-select';

const MODES: ActionMode[] = [
  'fully_autonomous', 'draft_and_approve', 'draft_only', 'context_only', 'drop',
];
const MATCH_TYPES = ['contact', 'domain', 'channel', 'keyword', 'default'] as const;

function modeLabel(m: string) {
  return m.replace(/_/g, ' ');
}

const MODE_OPTIONS = MODES.map(m => ({ value: m, label: modeLabel(m) }));
const MATCH_OPTIONS = MATCH_TYPES.map(t => ({ value: t, label: t }));

export function AutomationTab({ agentId }: { agentId: string }) {
  const [cfg, setCfg] = useState<InboundConfig | null>(null);
  const [rules, setRules] = useState<InboundRule[]>([]);
  const [saving, setSaving] = useState(false);
  const [showRuleForm, setShowRuleForm] = useState(false);
  const [newRule, setNewRule] = useState<Pick<InboundRule, 'priority' | 'match_type' | 'match_value' | 'mode'>>({
    priority: 100,
    match_type: 'contact',
    match_value: '',
    mode: 'draft_and_approve',
  });

  useEffect(() => {
    getInboundConfig(agentId).then(setCfg).catch(() => {});
    listInboundRules(agentId).then(setRules).catch(() => {});
  }, [agentId]);

  const saveCfg = async () => {
    if (!cfg) return;
    setSaving(true);
    try {
      await putInboundConfig(agentId, cfg);
      toast.success('Automation settings saved');
    } catch {
      toast.error('Could not save settings. Please try again.');
    } finally {
      setSaving(false);
    }
  };

  const addRule = async () => {
    if (!newRule.match_value && newRule.match_type !== 'default') {
      toast.error('Match value is required');
      return;
    }
    try {
      await createInboundRule(agentId, newRule);
      const fresh = await listInboundRules(agentId);
      setRules(fresh);
      setShowRuleForm(false);
      setNewRule({ priority: 100, match_type: 'contact', match_value: '', mode: 'draft_and_approve' });
      toast.success('Rule added');
    } catch {
      toast.error('Failed to add rule');
    }
  };

  const removeRule = async (ruleId: string) => {
    try {
      await deleteInboundRule(agentId, ruleId);
      setRules((r) => r.filter((x) => x.id !== ruleId));
    } catch {
      toast.error('Failed to delete rule');
    }
  };

  if (!cfg) {
    return <div className="p-6 text-sm text-muted-foreground">Loading...</div>;
  }

  return (
    <div className="p-6 space-y-8 max-w-2xl mx-auto">
      {/* Default behaviour */}
      <section>
        <h3 className="text-sm font-semibold mb-3">Default Behaviour</h3>
        <div className="space-y-3">
          <label className="block">
            <span className="text-xs text-muted-foreground">Known senders (default mode)</span>
            <div className="mt-1">
              <SearchableSelect
                value={cfg.default_mode}
                onChange={(v) => setCfg({ ...cfg, default_mode: v as ActionMode })}
                options={MODE_OPTIONS}
              />
            </div>
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Unknown senders</span>
            <div className="mt-1">
              <SearchableSelect
                value={cfg.unknown_sender_mode}
                onChange={(v) => setCfg({ ...cfg, unknown_sender_mode: v as ActionMode })}
                options={MODE_OPTIONS}
              />
            </div>
          </label>
          <label className="block">
            <span className="text-xs text-muted-foreground">Spam action</span>
            <div className="mt-1">
              <SearchableSelect
                value={cfg.spam_action}
                onChange={(v) => setCfg({ ...cfg, spam_action: v as 'drop' | 'context_only' })}
                options={[
                  { value: 'drop', label: 'Drop silently' },
                  { value: 'context_only', label: 'Context only (log it)' },
                ]}
              />
            </div>
          </label>
        </div>
      </section>

      {/* Notification channel */}
      <section>
        <h3 className="text-sm font-semibold mb-3">Approval Notifications</h3>
        <div className="space-y-3">
          <label className="block">
            <span className="text-xs text-muted-foreground">Push notifications via</span>
            <div className="mt-1">
              <SearchableSelect
                value={cfg.notification_channel || ''}
                onChange={(v) => setCfg({ ...cfg, notification_channel: v })}
                options={[
                  { value: '', label: 'None (web panel only)' },
                  { value: 'telegram', label: 'Telegram' },
                  { value: 'wechat', label: 'WeChat Work' },
                  { value: 'dingtalk', label: 'DingTalk' },
                ]}
              />
            </div>
          </label>
          {cfg.notification_channel && (
            <label className="block">
              <span className="text-xs text-muted-foreground">Channel target (bot token or chat ID)</span>
              <input
                type="text"
                value={cfg.notification_target}
                onChange={(e) => setCfg({ ...cfg, notification_target: e.target.value })}
                placeholder="e.g. 123456789"
                className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
              />
            </label>
          )}
        </div>
      </section>

      {/* Daily briefing */}
      <section>
        <h3 className="text-sm font-semibold mb-3">Daily Briefing</h3>
        <div className="space-y-3">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={cfg.briefing_enabled}
              onChange={(e) => setCfg({ ...cfg, briefing_enabled: e.target.checked })}
              className="rounded"
            />
            Enable morning briefing digest
          </label>
          {cfg.briefing_enabled && (
            <div className="flex gap-3">
              <label className="block flex-1">
                <span className="text-xs text-muted-foreground">Time (HH:MM)</span>
                <input
                  type="text"
                  value={cfg.briefing_time}
                  onChange={(e) => setCfg({ ...cfg, briefing_time: e.target.value })}
                  placeholder="08:00"
                  className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
                />
              </label>
              <label className="block flex-1">
                <span className="text-xs text-muted-foreground">Timezone</span>
                <input
                  type="text"
                  value={cfg.briefing_timezone}
                  onChange={(e) => setCfg({ ...cfg, briefing_timezone: e.target.value })}
                  placeholder="Asia/Shanghai"
                  className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
                />
              </label>
            </div>
          )}
        </div>
      </section>

      <button
        onClick={saveCfg}
        disabled={saving}
        className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
      >
        <Save className="h-3.5 w-3.5" />
        {saving ? 'Saving...' : 'Save Settings'}
      </button>

      {/* Routing rules */}
      <section>
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold">Routing Rules</h3>
          <button
            onClick={() => setShowRuleForm(true)}
            className="flex items-center gap-1 rounded-lg border border-border px-2.5 py-1 text-xs hover:bg-accent"
          >
            <Plus className="h-3 w-3" /> Add Rule
          </button>
        </div>

        {showRuleForm && (
          <div className="mb-4 rounded-xl border border-border bg-card p-4 space-y-3">
            <div className="grid grid-cols-2 gap-3">
              <label className="block">
                <span className="text-xs text-muted-foreground">Priority (lower = higher priority)</span>
                <input
                  type="number"
                  value={newRule.priority}
                  onChange={(e) => setNewRule({ ...newRule, priority: +e.target.value })}
                  className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
                />
              </label>
              <label className="block">
                <span className="text-xs text-muted-foreground">Match type</span>
                <div className="mt-1">
                  <SearchableSelect
                    value={newRule.match_type}
                    onChange={(v) => setNewRule({ ...newRule, match_type: v as InboundRule['match_type'] })}
                    options={MATCH_OPTIONS}
                  />
                </div>
              </label>
            </div>
            {newRule.match_type !== 'default' && (
              <label className="block">
                <span className="text-xs text-muted-foreground">Match value</span>
                <input
                  type="text"
                  value={newRule.match_value}
                  onChange={(e) => setNewRule({ ...newRule, match_value: e.target.value })}
                  placeholder={
                    newRule.match_type === 'contact' ? 'buyer@acme.com' :
                    newRule.match_type === 'domain' ? '@acme.com' :
                    newRule.match_type === 'channel' ? 'telegram' :
                    'price|invoice|payment'
                  }
                  className="mt-1 block w-full rounded-lg border border-border bg-background px-3 py-1.5 text-sm"
                />
              </label>
            )}
            <label className="block">
              <span className="text-xs text-muted-foreground">Action mode</span>
              <div className="mt-1">
                <SearchableSelect
                  value={newRule.mode}
                  onChange={(v) => setNewRule({ ...newRule, mode: v })}
                  options={MODE_OPTIONS}
                />
              </div>
            </label>
            <div className="flex gap-2">
              <button onClick={addRule} className="rounded-lg bg-primary px-3 py-1.5 text-xs text-primary-foreground hover:bg-primary/90">
                Add Rule
              </button>
              <button onClick={() => setShowRuleForm(false)} className="rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent">
                Cancel
              </button>
            </div>
          </div>
        )}

        {rules.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No rules yet. All messages use the default mode above.
          </p>
        ) : (
          <div className="rounded-xl border border-border overflow-hidden">
            <table className="w-full text-xs">
              <thead className="bg-muted/30">
                <tr>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Priority</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Match</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Value</th>
                  <th className="px-3 py-2 text-left font-medium text-muted-foreground">Mode</th>
                  <th className="px-3 py-2" />
                </tr>
              </thead>
              <tbody>
                {rules.map((r) => (
                  <tr key={r.id} className="border-t border-border">
                    <td className="px-3 py-2 font-mono">{r.priority}</td>
                    <td className="px-3 py-2 text-muted-foreground">{r.match_type}</td>
                    <td className="px-3 py-2 font-mono truncate max-w-[120px]">{r.match_value || '—'}</td>
                    <td className="px-3 py-2">{modeLabel(r.mode)}</td>
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
    </div>
  );
}
