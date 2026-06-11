'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Check, RotateCcw, Loader2, AlertTriangle } from 'lucide-react';
import type { ProjectArtifact, ResourcePlan } from '@/types';
import { cn } from '@/lib/utils';

function uusdToDollars(uusd: number): string {
  return `$${(uusd / 1_000_000).toFixed(2)}`;
}

function parseResourcePlan(content_md: string): ResourcePlan | null {
  try {
    const marker = '<!-- RESOURCE_PLAN_JSON';
    const start = content_md.indexOf(marker);
    if (start === -1) return null;
    const jsonStart = content_md.indexOf('{', start + marker.length);
    if (jsonStart === -1) return null;
    const end = content_md.indexOf('-->', jsonStart);
    if (end === -1) return null;
    const raw = content_md.slice(jsonStart, end).trim();
    return JSON.parse(raw) as ResourcePlan;
  } catch {
    return null;
  }
}

export function ResourcePlanView({ artifact, busy, onApprove, onRequestChanges }: {
  artifact: ProjectArtifact;
  busy: boolean;
  onApprove: () => void;
  onRequestChanges: (feedback: string) => void;
}) {
  const [feedback, setFeedback] = useState('');
  const [showFeedback, setShowFeedback] = useState(false);

  const plan = parseResourcePlan(artifact.content_md);
  const approved = artifact.status === 'approved';

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-2.5">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">Resource Plan</span>
          <span className="text-2xs text-muted-foreground">v{artifact.version}</span>
          <span className={cn('rounded-full px-2 py-0.5 text-2xs font-medium',
            approved
              ? 'bg-emerald-500/10 text-emerald-500'
              : artifact.status === 'needs_review'
              ? 'bg-amber-500/10 text-amber-500'
              : 'bg-muted text-muted-foreground')}>
            {artifact.status}
          </span>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto px-5 py-4 space-y-5">
        {plan == null ? (
          /* Fallback: render raw markdown if parse fails */
          <pre className="whitespace-pre-wrap break-words text-2sm leading-relaxed font-sans">
            {artifact.content_md}
          </pre>
        ) : (
          <>
            {/* Summary row */}
            <div className="grid grid-cols-3 gap-3">
              <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <p className="text-2xs text-muted-foreground mb-0.5">Total estimate</p>
                <p className="text-sm font-semibold">{uusdToDollars(plan.total_est_uusd)}</p>
              </div>
              <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <p className="text-2xs text-muted-foreground mb-0.5">Project cap</p>
                <p className="text-sm font-semibold">{uusdToDollars(plan.project_cap_uusd)}</p>
              </div>
              <div className="rounded-lg border border-border bg-muted/40 px-3 py-2">
                <p className="text-2xs text-muted-foreground mb-0.5">Timeline</p>
                <p className="text-sm font-semibold">{plan.timeline}</p>
              </div>
            </div>

            {/* Agent table */}
            <div className="overflow-x-auto rounded-lg border border-border">
              <table className="w-full text-2sm">
                <thead>
                  <tr className="border-b border-border bg-muted/60">
                    <th className="px-3 py-2 text-left font-medium text-muted-foreground">Role</th>
                    <th className="px-3 py-2 text-left font-medium text-muted-foreground">Model</th>
                    <th className="px-3 py-2 text-left font-medium text-muted-foreground">Provider</th>
                    <th className="px-3 py-2 text-right font-medium text-muted-foreground">Tokens in</th>
                    <th className="px-3 py-2 text-right font-medium text-muted-foreground">Tokens out</th>
                    <th className="px-3 py-2 text-right font-medium text-muted-foreground">Cap</th>
                  </tr>
                </thead>
                <tbody>
                  {plan.agents.map((agent, i) => (
                    <tr key={i} className="border-b border-border last:border-0 hover:bg-muted/30 transition-colors">
                      <td className="px-3 py-2 font-medium">{agent.role}</td>
                      <td className="px-3 py-2 text-muted-foreground">{agent.model_id}</td>
                      <td className="px-3 py-2 text-muted-foreground">{agent.provider_id}</td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {agent.est_tokens_in.toLocaleString()}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {agent.est_tokens_out.toLocaleString()}
                      </td>
                      <td className="px-3 py-2 text-right">
                        {agent.pricing_known
                          ? uusdToDollars(agent.cap_uusd)
                          : <span className="text-muted-foreground italic">pricing unknown</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            {/* Notes */}
            {plan.notes && plan.notes.length > 0 && (
              <div className="space-y-1.5">
                {plan.notes.map((note, i) => (
                  <div key={i} className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 px-3 py-2">
                    <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5 text-amber-500" />
                    <p className="text-2sm text-muted-foreground">{note}</p>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {/* Approve / Request-changes bar — mirrors ArtifactPane */}
      {!approved && (
        <div className="shrink-0 border-t border-border p-3 space-y-2">
          {showFeedback ? (
            <div className="space-y-2">
              <textarea
                value={feedback}
                onChange={e => setFeedback(e.target.value)}
                placeholder="What should change?"
                className="qr-textarea text-2sm"
                rows={3}
              />
              <div className="flex gap-2">
                <button
                  onClick={() => { onRequestChanges(feedback); setShowFeedback(false); setFeedback(''); }}
                  disabled={busy || !feedback.trim()}
                  className="qr-btn qr-btn-primary qr-btn-sm">
                  Send changes
                </button>
                <button onClick={() => setShowFeedback(false)} className="qr-btn qr-btn-ghost qr-btn-sm">
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <div className="flex gap-2">
              <button onClick={onApprove} disabled={busy} className="qr-btn qr-btn-primary qr-btn-sm flex-1">
                {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />} Approve
              </button>
              <button onClick={() => setShowFeedback(true)} disabled={busy} className="qr-btn qr-btn-outline qr-btn-sm">
                <RotateCcw className="h-3.5 w-3.5" /> Request changes
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
