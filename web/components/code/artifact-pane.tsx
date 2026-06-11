'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Check, RotateCcw, Loader2, FileText } from 'lucide-react';
import type { ProjectArtifact, ArtifactType } from '@/types';
import { cn } from '@/lib/utils';

const ARTIFACT_LABELS: Record<ArtifactType, string> = {
  prd: 'PRD', spec: 'Tech Spec', design: 'System Design', resource_plan: 'Resource Plan',
};

export function ArtifactPane({ artifact, busy, onApprove, onRequestChanges }: {
  artifact: ProjectArtifact | null;
  busy: boolean;
  onApprove: () => void;
  onRequestChanges: (feedback: string) => void;
}) {
  const [feedback, setFeedback] = useState('');
  const [showFeedback, setShowFeedback] = useState(false);

  if (!artifact) {
    return (
      <div className="flex h-full flex-col items-center justify-center text-muted-foreground gap-2">
        <FileText className="h-8 w-8 opacity-40" />
        <p className="text-sm">The document appears here as the CTO drafts it.</p>
      </div>
    );
  }

  const approved = artifact.status === 'approved';
  return (
    <div className="flex h-full flex-col">
      <div className="flex shrink-0 items-center justify-between border-b border-border px-4 py-2.5">
        <div className="flex items-center gap-2">
          <span className="text-sm font-semibold">{ARTIFACT_LABELS[artifact.type]}</span>
          <span className="text-2xs text-muted-foreground">v{artifact.version}</span>
          <span className={cn('rounded-full px-2 py-0.5 text-2xs font-medium',
            approved ? 'bg-emerald-500/10 text-emerald-500'
              : artifact.status === 'needs_review' ? 'bg-amber-500/10 text-amber-500'
              : 'bg-muted text-muted-foreground')}>
            {artifact.status}
          </span>
        </div>
      </div>
      <div className="flex-1 overflow-y-auto px-5 py-4">
        <pre className="whitespace-pre-wrap break-words text-2sm leading-relaxed font-sans">{artifact.content_md}</pre>
      </div>
      {!approved && (
        <div className="shrink-0 border-t border-border p-3 space-y-2">
          {showFeedback ? (
            <div className="space-y-2">
              <textarea value={feedback} onChange={e => setFeedback(e.target.value)}
                placeholder="What should change?" className="qr-textarea text-2sm" rows={3} />
              <div className="flex gap-2">
                <button onClick={() => { onRequestChanges(feedback); setShowFeedback(false); setFeedback(''); }}
                  disabled={busy || !feedback.trim()}
                  className="qr-btn qr-btn-primary qr-btn-sm">Send changes</button>
                <button onClick={() => setShowFeedback(false)} className="qr-btn qr-btn-ghost qr-btn-sm">Cancel</button>
              </div>
            </div>
          ) : (
            <div className="flex gap-2">
              <button onClick={onApprove} disabled={busy}
                className="qr-btn qr-btn-primary qr-btn-sm flex-1">
                {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />} Approve
              </button>
              <button onClick={() => setShowFeedback(true)} disabled={busy}
                className="qr-btn qr-btn-outline qr-btn-sm">
                <RotateCcw className="h-3.5 w-3.5" /> Request changes
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
