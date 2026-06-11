'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState, useCallback } from 'react';
import { Send, Loader2 } from 'lucide-react';
import { projectBriefs as api } from '@/lib/api';
import type { ProjectArtifact, ArtifactType } from '@/types';
import { ArtifactPane } from './artifact-pane';
import { ResourcePlanView } from './resource-plan-view';
import { BudgetBar } from './budget-bar';
import { cn } from '@/lib/utils';

const STAGE_ARTIFACT: ArtifactType[] = ['prd', 'spec', 'design', 'resource_plan'];

const STAGE_LABELS: Record<ArtifactType, string> = {
  prd: 'PRD',
  spec: 'Spec',
  design: 'Design',
  resource_plan: 'Resource Plan',
};

/** Returns the ArtifactType whose approval is required before generating `type`. */
const GENERATE_GATE: Partial<Record<ArtifactType, ArtifactType>> = {
  spec: 'prd',
  design: 'spec',
  resource_plan: 'design',
};

type Msg = { role: 'assistant' | 'user'; content: string };

export function OrgPipeline({ briefId }: { briefId: string }) {
  const [stage, setStage] = useState('intake');
  const [artifacts, setArtifacts] = useState<ProjectArtifact[]>([]);
  const [active, setActive] = useState<ArtifactType | null>(null);
  const [messages, setMessages] = useState<Msg[]>([]);
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    const r = await api.artifacts(briefId);
    setStage(r.stage);
    setArtifacts(r.artifacts);
    const cur = r.artifacts.find(a => a.type === r.stage) ?? r.artifacts[r.artifacts.length - 1] ?? null;
    if (cur) setActive(cur.type as ArtifactType);
  }, [briefId]);

  useEffect(() => { refresh(); }, [refresh]);

  const send = async () => {
    if (!input.trim()) return;
    const msg = input.trim();
    setMessages(m => [...m, { role: 'user', content: msg }]);
    setInput(''); setBusy(true);
    try {
      const r = await api.clarify(briefId, msg, messages.map(m => ({ role: m.role, content: m.content })));
      setMessages(m => [...m, { role: 'assistant', content: r.reply }]);
      await refresh();
    } finally { setBusy(false); }
  };

  const generate = async (type: ArtifactType) => {
    setBusy(true);
    try { const a = await api.generate(briefId, type); setActive(type); setArtifacts(prev => [...prev.filter(p => p.type !== type), a]); }
    finally { setBusy(false); }
  };

  const activeArtifact = artifacts.find(a => a.type === active) ?? null;

  const onApprove = async () => {
    setBusy(true);
    try { await api.approveArtifact(briefId, active!); await refresh(); }
    finally { setBusy(false); }
  };

  const onRequestChanges = async (fb: string) => {
    setBusy(true);
    try { await api.requestChanges(briefId, active!, fb); await refresh(); }
    finally { setBusy(false); }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Pipeline header with burn meter */}
      <div className="shrink-0 border-b border-border px-4 py-1.5">
        <BudgetBar projectId={briefId} />
      </div>

      <div className="flex flex-1 min-h-0">
        {/* Left: clarify chat */}
        <div className="flex w-[420px] shrink-0 flex-col border-r border-border">
          <div className="flex-1 overflow-y-auto px-4 py-3 space-y-3">
            {messages.length === 0 && (
              <p className="text-2sm text-muted-foreground">The CTO will ask a few questions, then draft your PRD. Describe anything else it should know.</p>
            )}
            {messages.map((m, i) => (
              <div key={i} className={cn('text-2sm', m.role === 'user' ? 'text-foreground' : 'text-muted-foreground')}>
                <span className="font-semibold">{m.role === 'user' ? 'You' : 'CTO'}: </span>{m.content}
              </div>
            ))}
          </div>
          <div className="shrink-0 border-t border-border p-3 space-y-2">
            <div className="flex flex-wrap gap-1.5">
              {STAGE_ARTIFACT.map(t => {
                const a = artifacts.find(x => x.type === t);
                const isApproved = a?.status === 'approved';
                const gateType = GENERATE_GATE[t];
                const gateApproved = gateType
                  ? artifacts.find(x => x.type === gateType)?.status === 'approved'
                  : true;
                return (
                  <button key={t} onClick={() => generate(t)}
                    disabled={busy || isApproved || !gateApproved}
                    title={!gateApproved && gateType ? `Requires ${STAGE_LABELS[gateType]} approval first` : undefined}
                    className="qr-btn qr-btn-outline qr-btn-xs">
                    {STAGE_LABELS[t]}
                  </button>
                );
              })}
            </div>
            <div className="flex gap-2">
              <input value={input} onChange={e => setInput(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && send()} placeholder="Reply to the CTO…"
                className="qr-input text-2sm flex-1" />
              <button onClick={send} disabled={busy} className="qr-btn qr-btn-primary qr-btn-icon">
                {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
              </button>
            </div>
          </div>
        </div>

        {/* Right: artifact review */}
        <div className="flex-1 min-w-0">
          {active === 'resource_plan' && activeArtifact ? (
            <ResourcePlanView artifact={activeArtifact} busy={busy}
              onApprove={onApprove}
              onRequestChanges={onRequestChanges} />
          ) : (
            <ArtifactPane artifact={activeArtifact} busy={busy}
              onApprove={onApprove}
              onRequestChanges={onRequestChanges} />
          )}
        </div>
      </div>
    </div>
  );
}
