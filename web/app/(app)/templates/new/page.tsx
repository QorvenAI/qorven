'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import {
  Plus, Trash2, ArrowLeft, Loader2, Sparkles, Users,
  BarChart3, Save, Check, Bot, Layout,
} from 'lucide-react';
import { CanvasHeader } from '@/components/layouts/canvas-header';
import { workspaces } from '@/lib/api';
import { request } from '@/lib/api-core';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

// ─── Visual Workspace Builder ────────────────────────────────────────────────
// Lets users create custom workspaces without describing to an AI.
// Drag-and-drop agents and blocks to assemble a workspace from scratch.

const BLOCK_TYPES = [
  { type: 'stat-row', label: 'Stats Row', icon: '📊' },
  { type: 'chart', label: 'Chart', icon: '📈', extra: { chartType: 'bar' } },
  { type: 'data-table', label: 'Data Table', icon: '📋', extra: { searchable: true } },
  { type: 'pipeline', label: 'Pipeline', icon: '🔄' },
  { type: 'kanban', label: 'Kanban Board', icon: '🗂️' },
  { type: 'feed', label: 'Activity Feed', icon: '📝' },
  { type: 'calendar', label: 'Calendar', icon: '📅' },
  { type: 'contacts', label: 'Contacts', icon: '👥' },
  { type: 'timeline', label: 'Timeline', icon: '⏱️' },
  { type: 'markdown', label: 'Markdown', icon: '✍️' },
];

const LAYOUTS = [
  { id: 'grid-2col', label: '2 Columns', preview: '⬜⬜' },
  { id: 'grid-3col', label: '3 Columns', preview: '⬜⬜⬜' },
  { id: 'sidebar-right', label: 'Sidebar Right', preview: '⬛⬜' },
  { id: 'sidebar-left', label: 'Sidebar Left', preview: '⬜⬛' },
];

const AGENT_ROLES = ['leader', 'specialist', 'researcher', 'developer', 'writer', 'analyst', 'support'];
const TOOL_PROFILES = [
  { id: 'full', label: 'Full Access' },
  { id: 'minimal', label: 'Chat Only' },
  { id: 'code', label: 'Developer' },
];

type AgentDef = { key: string; name: string; role: string; system_prompt: string; reports_to?: string; tool_profile: string };
type BlockDef = { type: string; title: string; [k: string]: any };

export default function NewWorkspacePage() {
  const router = useRouter();
  const [step, setStep] = useState<'meta' | 'agents' | 'dashboard' | 'review'>('meta');
  const [saving, setSaving] = useState(false);

  // Workspace metadata
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [icon, setIcon] = useState('🚀');
  const [category, setCategory] = useState('business');

  // Agents
  const [agents, setAgents] = useState<AgentDef[]>([
    { key: 'lead', name: 'Lead Agent', role: 'leader', system_prompt: 'You coordinate the team and manage priorities.', tool_profile: 'full' },
  ]);

  // Dashboard
  const [layout, setLayout] = useState('grid-2col');
  const [blocks, setBlocks] = useState<BlockDef[]>([
    { type: 'stat-row', title: 'Stats' },
    { type: 'feed', title: 'Activity' },
  ]);

  const addAgent = () => setAgents(prev => [...prev, {
    key: `agent-${prev.length + 1}`, name: `Agent ${prev.length + 1}`,
    role: 'specialist', system_prompt: 'You are a helpful specialist.', tool_profile: 'full',
  }]);

  const removeAgent = (idx: number) => setAgents(prev => prev.filter((_, i) => i !== idx));

  const updateAgent = (idx: number, field: keyof AgentDef, value: string) =>
    setAgents(prev => prev.map((a, i) => i === idx ? { ...a, [field]: value } : a));

  const addBlock = (type: string, extra?: Record<string, any>) =>
    setBlocks(prev => [...prev, { type, title: BLOCK_TYPES.find(b => b.type === type)?.label ?? type, ...extra }]);

  const removeBlock = (idx: number) => setBlocks(prev => prev.filter((_, i) => i !== idx));

  const updateBlock = (idx: number, field: string, value: string) =>
    setBlocks(prev => prev.map((b, i) => i === idx ? { ...b, [field]: value } : b));

  const save = async () => {
    if (!name.trim()) { toast.error('Workspace name required'); return; }
    if (agents.length === 0) { toast.error('At least one agent required'); return; }
    setSaving(true);
    try {
      // Install the custom workspace via self-build endpoint
      const result = await workspaces.selfBuild(
        `Custom workspace: ${name}. ${description}`,
      ) as any;

      // The self-build created a workspace using template matching.
      // Now save the custom dashboard on top of it.
      const templateId = result?.template_id ?? `custom-${Date.now()}`;
      await request<any>('/dashboards', {
        method: 'POST',
        body: JSON.stringify({ template_id: templateId, name, config: { layout, blocks } }),
      });

      toast.success(`"${name}" workspace created!`);
      router.push(`/dashboard/${templateId}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Failed to create workspace');
    } finally { setSaving(false); }
  };

  const ICONS = ['🚀', '📊', '🎯', '💼', '🏢', '⚡', '🔥', '🌟', '🎨', '🛠️', '📈', '🤝', '💡', '🔬', '🎓'];
  const CATEGORIES = ['business', 'marketing', 'engineering', 'analytics', 'education', 'professional'];

  const steps = [
    { id: 'meta', label: 'Basics' },
    { id: 'agents', label: 'Agents' },
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'review', label: 'Review' },
  ];

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <CanvasHeader
        title="Custom Workspace Builder"
        description="Design your own AI team and dashboard"
        actions={
          <button onClick={() => router.back()}
            className="h-8 w-8 flex items-center justify-center rounded-lg text-muted-foreground hover:bg-accent cursor-pointer">
            <ArrowLeft className="h-4 w-4" />
          </button>
        }
      />

      {/* Step progress */}
      <div className="flex items-center gap-0">
        {steps.map((s, i) => (
          <div key={s.id} className="flex items-center flex-1">
            <button
              onClick={() => setStep(s.id as any)}
              className={cn(
                'flex items-center gap-1.5 text-xs font-medium transition-colors cursor-pointer',
                step === s.id ? 'text-primary' : steps.findIndex(x => x.id === step) > i ? 'text-emerald-500' : 'text-muted-foreground',
              )}>
              <span className={cn(
                'flex h-5 w-5 items-center justify-center rounded-full text-xs font-bold',
                step === s.id ? 'bg-primary text-primary-foreground' :
                steps.findIndex(x => x.id === step) > i ? 'bg-emerald-500 text-white' : 'bg-muted text-muted-foreground',
              )}>
                {steps.findIndex(x => x.id === step) > i ? <Check className="h-3 w-3" /> : i + 1}
              </span>
              {s.label}
            </button>
            {i < steps.length - 1 && <div className="flex-1 h-px bg-border mx-2" />}
          </div>
        ))}
      </div>

      {/* Step: Basics */}
      {step === 'meta' && (
        <div className="rounded-xl border border-border bg-card p-6 space-y-5">
          <h2 className="text-base font-semibold">Workspace Identity</h2>

          {/* Icon picker */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">Icon</label>
            <div className="flex flex-wrap gap-2 mt-2">
              {ICONS.map(i => (
                <button key={i} onClick={() => setIcon(i)}
                  className={cn('h-9 w-9 rounded-lg text-xl flex items-center justify-center border cursor-pointer transition-all',
                    icon === i ? 'border-primary bg-primary/10 scale-110' : 'border-border hover:border-primary/40')}>
                  {i}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Workspace Name *</label>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-2xl">{icon}</span>
              <input value={name} onChange={e => setName(e.target.value)} placeholder="e.g. Sales Team"
                className="qr-input flex-1" />
            </div>
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Description</label>
            <textarea value={description} onChange={e => setDescription(e.target.value)}
              placeholder="What does this workspace do?"
              rows={2} className="mt-1 qr-textarea resize-none" />
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Category</label>
            <div className="flex flex-wrap gap-2 mt-1">
              {CATEGORIES.map(cat => (
                <button key={cat} onClick={() => setCategory(cat)}
                  className={cn('rounded-lg border px-3 py-1.5 text-xs font-medium capitalize cursor-pointer transition-colors',
                    category === cat ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:bg-accent')}>
                  {cat}
                </button>
              ))}
            </div>
          </div>

          <button onClick={() => setStep('agents')} disabled={!name.trim()}
            className="w-full rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
            Next: Configure Agents →
          </button>
        </div>
      )}

      {/* Step: Agents */}
      {step === 'agents' && (
        <div className="rounded-xl border border-border bg-card p-6 space-y-5">
          <div className="flex items-center justify-between">
            <h2 className="text-base font-semibold">AI Agents ({agents.length})</h2>
            <button onClick={addAgent}
              className="flex items-center gap-1 rounded-lg border border-border px-3 py-1.5 text-xs hover:bg-accent cursor-pointer">
              <Plus className="h-3.5 w-3.5" /> Add Agent
            </button>
          </div>

          <div className="space-y-4">
            {agents.map((agent, idx) => (
              <div key={idx} className="rounded-xl border border-border p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Bot className={cn('h-4 w-4', agent.role === 'leader' ? 'text-primary' : 'text-muted-foreground')} />
                    <span className="text-sm font-medium">{agent.name || 'Unnamed Agent'}</span>
                  </div>
                  {agents.length > 1 && (
                    <button onClick={() => removeAgent(idx)} className="text-muted-foreground hover:text-destructive cursor-pointer">
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="text-xs text-muted-foreground">Name</label>
                    <input value={agent.name} onChange={e => updateAgent(idx, 'name', e.target.value)}
                      className="mt-0.5 qr-input text-xs" />
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground">Key (slug)</label>
                    <input value={agent.key} onChange={e => updateAgent(idx, 'key', e.target.value.toLowerCase().replace(/\s+/g, '-'))}
                      className="mt-0.5 qr-input text-xs font-mono" />
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground">Role</label>
                    <select value={agent.role} onChange={e => updateAgent(idx, 'role', e.target.value)}
                      className="mt-0.5 qr-input text-xs">
                      {AGENT_ROLES.map(r => <option key={r} value={r}>{r}</option>)}
                    </select>
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground">Tools</label>
                    <select value={agent.tool_profile} onChange={e => updateAgent(idx, 'tool_profile', e.target.value)}
                      className="mt-0.5 qr-input text-xs">
                      {TOOL_PROFILES.map(p => <option key={p.id} value={p.id}>{p.label}</option>)}
                    </select>
                  </div>
                </div>
                <div>
                  <label className="text-xs text-muted-foreground">System Prompt</label>
                  <textarea value={agent.system_prompt} onChange={e => updateAgent(idx, 'system_prompt', e.target.value)}
                    rows={2} className="mt-0.5 qr-textarea text-xs resize-none" />
                </div>
                {agents.length > 1 && idx > 0 && (
                  <div>
                    <label className="text-xs text-muted-foreground">Reports to</label>
                    <select value={agent.reports_to ?? ''} onChange={e => updateAgent(idx, 'reports_to', e.target.value)}
                      className="mt-0.5 qr-input text-xs">
                      <option value="">None (top-level)</option>
                      {agents.filter((_, i) => i !== idx).map(a => (
                        <option key={a.key} value={a.key}>{a.name}</option>
                      ))}
                    </select>
                  </div>
                )}
              </div>
            ))}
          </div>

          <div className="flex gap-2">
            <button onClick={() => setStep('meta')} className="flex-1 rounded-lg border border-border py-2.5 text-sm hover:bg-accent cursor-pointer">← Back</button>
            <button onClick={() => setStep('dashboard')} className="flex-1 rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 cursor-pointer">Next: Dashboard →</button>
          </div>
        </div>
      )}

      {/* Step: Dashboard */}
      {step === 'dashboard' && (
        <div className="rounded-xl border border-border bg-card p-6 space-y-5">
          <h2 className="text-base font-semibold">Dashboard Layout</h2>

          {/* Layout selector */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">Layout</label>
            <div className="flex gap-2 mt-2">
              {LAYOUTS.map(l => (
                <button key={l.id} onClick={() => setLayout(l.id)}
                  className={cn('flex-1 rounded-lg border px-3 py-2 text-xs text-center cursor-pointer transition-colors',
                    layout === l.id ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:bg-accent')}>
                  <div className="text-base mb-1">{l.preview}</div>
                  {l.label}
                </button>
              ))}
            </div>
          </div>

          {/* Blocks */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-xs font-medium text-muted-foreground">Widgets ({blocks.length})</label>
            </div>
            <div className="flex flex-wrap gap-1.5 mb-3">
              {BLOCK_TYPES.map(b => (
                <button key={b.type} onClick={() => addBlock(b.type, b.extra)}
                  className="flex items-center gap-1 rounded-lg border border-dashed border-border px-2.5 py-1.5 text-xs text-muted-foreground hover:border-primary/40 hover:text-foreground cursor-pointer transition-colors">
                  <Plus className="h-3 w-3" /> {b.icon} {b.label}
                </button>
              ))}
            </div>
            <div className="space-y-1.5">
              {blocks.map((block, idx) => (
                <div key={idx} className="flex items-center gap-2 rounded-lg border border-border bg-input px-3 py-2">
                  <span className="text-sm">{BLOCK_TYPES.find(b => b.type === block.type)?.icon ?? '📦'}</span>
                  <span className="text-xs text-muted-foreground font-mono w-24 shrink-0">{block.type}</span>
                  <input value={block.title} onChange={e => updateBlock(idx, 'title', e.target.value)}
                    placeholder="Widget title" className="flex-1 bg-transparent text-xs outline-none" />
                  <button onClick={() => removeBlock(idx)} className="text-muted-foreground hover:text-destructive cursor-pointer shrink-0">
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </div>
              ))}
            </div>
          </div>

          <div className="flex gap-2">
            <button onClick={() => setStep('agents')} className="flex-1 rounded-lg border border-border py-2.5 text-sm hover:bg-accent cursor-pointer">← Back</button>
            <button onClick={() => setStep('review')} className="flex-1 rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 cursor-pointer">Review →</button>
          </div>
        </div>
      )}

      {/* Step: Review */}
      {step === 'review' && (
        <div className="rounded-xl border border-border bg-card p-6 space-y-5">
          <h2 className="text-base font-semibold">Review & Launch</h2>

          <div className="rounded-xl border border-border bg-muted/20 p-4 space-y-3">
            <div className="flex items-center gap-2">
              <span className="text-2xl">{icon}</span>
              <div>
                <p className="font-semibold">{name}</p>
                <p className="text-xs text-muted-foreground capitalize">{category}</p>
              </div>
            </div>
            {description && <p className="text-sm text-muted-foreground">{description}</p>}
          </div>

          <div className="grid grid-cols-2 gap-3">
            <div className="rounded-xl border border-border p-3 text-center">
              <Users className="h-5 w-5 mx-auto mb-1 text-primary" />
              <p className="text-lg font-bold">{agents.length}</p>
              <p className="text-xs text-muted-foreground">Agents</p>
            </div>
            <div className="rounded-xl border border-border p-3 text-center">
              <Layout className="h-5 w-5 mx-auto mb-1 text-primary" />
              <p className="text-lg font-bold">{blocks.length}</p>
              <p className="text-xs text-muted-foreground">Widgets</p>
            </div>
          </div>

          <div className="space-y-1">
            {agents.map((a, i) => (
              <div key={i} className="flex items-center gap-2 text-xs text-muted-foreground">
                <Bot className="h-3.5 w-3.5 shrink-0" />
                <span className="font-medium text-foreground">{a.name}</span>
                <span className="text-muted-foreground">({a.role})</span>
                {a.reports_to && <span>→ reports to {agents.find(ag => ag.key === a.reports_to)?.name}</span>}
              </div>
            ))}
          </div>

          <div className="flex gap-2">
            <button onClick={() => setStep('dashboard')} className="flex-1 rounded-lg border border-border py-2.5 text-sm hover:bg-accent cursor-pointer">← Back</button>
            <button onClick={save} disabled={saving}
              className="flex-1 flex items-center justify-center gap-1.5 rounded-lg bg-primary text-primary-foreground py-2.5 text-sm font-medium hover:bg-primary/90 disabled:opacity-50 cursor-pointer">
              {saving ? <><Loader2 className="h-4 w-4 animate-spin" /> Creating…</> : <><Sparkles className="h-4 w-4" /> Launch Workspace</>}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
