'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useState } from 'react';
import { agents, request } from '@/lib/api';
import { useStore } from '@/store';
import { useSelectedModels } from '@/hooks/use-selected-models';
import { X, Sparkles, Loader2 } from 'lucide-react';

const ROLE_OPTIONS = [
  { value: '',           label: 'General purpose' },
  { value: 'chief',      label: 'Chief of Staff' },
  { value: 'developer',  label: 'Developer' },
  { value: 'researcher', label: 'Researcher' },
  { value: 'writer',     label: 'Writer / Copywriter' },
  { value: 'analyst',    label: 'Data Analyst' },
  { value: 'worker',     label: 'Worker / Automation' },
];

export function CreateSoulSheet({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [name, setName] = useState('');
  const [key, setKey] = useState('');
  const [role, setRole] = useState('');
  const [model, setModel] = useState('');
  const [description, setDescription] = useState('');
  const [prompt, setPrompt] = useState('');
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const setSouls = useStore((s) => s.setSouls);
  const { models, defaultModel } = useSelectedModels();

  if (!model && defaultModel) setModel(defaultModel);
  if (!open) return null;

  const autoKey = (n: string) => n.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

  const handleNameChange = (v: string) => {
    setName(v);
    if (!key || key === autoKey(name)) setKey(autoKey(v));
  };

  const generateSoul = async () => {
    if (!name) return;
    setGenerating(true);
    try {
      const res = await request<{ soul: string }>('/agents/generate-soul', {
        method: 'POST',
        body: JSON.stringify({ name, role, description }),
      });
      setPrompt(res.soul ?? '');
    } catch {
      // fallback: leave prompt as-is
    } finally {
      setGenerating(false);
    }
  };

  const handleSave = async () => {
    if (!name || !key) return;
    setSaving(true);
    try {
      await agents.create({
        display_name: name,
        agent_key: key,
        role,
        model: model || defaultModel,
        system_prompt: prompt,
      });
      const list = await agents.list();
      setSouls(list);
      onClose();
      setName(''); setKey(''); setRole(''); setDescription(''); setPrompt(''); setModel('');
    } catch {
      alert('Failed to create Qor');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40" onClick={onClose}>
      <div
        className="w-full max-w-md bg-background h-full overflow-y-auto border-l border-border p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold">Create New Qor</h2>
          <button onClick={onClose} className="h-8 w-8 flex items-center justify-center rounded-lg hover:bg-accent">
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4">
          <Field label="Name" value={name} onChange={handleNameChange} placeholder="e.g. Research Assistant" autoFocus />
          <Field label="Key" value={key} onChange={setKey} placeholder="e.g. researcher" note="Unique identifier — no spaces" />

          <div>
            <label className="text-xs font-medium text-muted-foreground">Role</label>
            <select value={role} onChange={(e) => setRole(e.target.value)} className="qr-select mt-1">
              {ROLE_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-xs font-medium text-muted-foreground">Model</label>
            {models.length === 0 ? (
              <p className="mt-1 text-xs text-muted-foreground">No models selected — go to Settings → Providers</p>
            ) : (
              <select value={model} onChange={(e) => setModel(e.target.value)} className="qr-select mt-1">
                {models.map((m) => (
                  <option key={m.model_id} value={m.model_id}>
                    {m.model_id}{m.is_default ? ' ★ Default' : ''}
                  </option>
                ))}
              </select>
            )}
          </div>

          {/* Description — seed for soul generation */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              What should this agent do? <span className="text-muted-foreground/60">(optional)</span>
            </label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              placeholder="e.g. Monitor competitor pricing daily and summarise changes into a report"
              className="qr-textarea mt-1 resize-none"
            />
          </div>

          {/* Soul / System Prompt */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-xs font-medium text-muted-foreground">Soul (system prompt)</label>
              <button
                onClick={generateSoul}
                disabled={!name || generating}
                className="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2 py-1 text-[11px] font-medium text-primary hover:bg-primary/20 disabled:opacity-40"
              >
                {generating
                  ? <><Loader2 className="h-3 w-3 animate-spin" /> Generating…</>
                  : <><Sparkles className="h-3 w-3" /> Generate with AI</>
                }
              </button>
            </div>
            <textarea
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              rows={6}
              placeholder="Describe this agent's identity, personality, and capabilities — or click Generate with AI above."
              className="qr-textarea resize-none"
            />
            {!prompt && (
              <p className="mt-1 text-[11px] text-muted-foreground">
                Leave blank to use role defaults, or fill the description above and click Generate with AI.
              </p>
            )}
          </div>

          <button
            onClick={handleSave}
            disabled={saving || !name || !key}
            className="w-full rounded-lg bg-primary py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {saving ? 'Creating…' : 'Create Qor'}
          </button>
        </div>
      </div>
    </div>
  );
}

function Field({
  label, value, onChange, placeholder, note, autoFocus,
}: {
  label: string; value: string; onChange: (v: string) => void;
  placeholder: string; note?: string; autoFocus?: boolean;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-muted-foreground">{label}</label>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        autoFocus={autoFocus}
        className="mt-1 qr-input"
      />
      {note && <p className="mt-0.5 text-[11px] text-muted-foreground">{note}</p>}
    </div>
  );
}
