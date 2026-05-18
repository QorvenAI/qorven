'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState } from 'react';
import { Mail, Check, X, Edit3 } from 'lucide-react';
import { confirmInboundRule, discardInboundRule } from '@/lib/api-inbound';
import { agents } from '@/lib/api-agents';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

// Parses the #!qorven:mail_rule_card payload emitted by set_mail_rule tool.
function parseRuleCard(text: string): Record<string, string> {
  const lines = text.split('\n');
  const result: Record<string, string> = {};
  for (const line of lines) {
    const idx = line.indexOf('=');
    if (idx > 0) {
      result[line.slice(0, idx).trim()] = line.slice(idx + 1).trim();
    }
  }
  return result;
}

function parsePolicyCard(text: string): Record<string, string> {
  return parseRuleCard(text);
}

const MODE_LABELS: Record<string, string> = {
  fully_autonomous: 'Auto-reply',
  draft_and_approve: 'Draft & hold for approval',
  draft_only: 'Draft only (no send)',
  context_only: 'Log only',
  drop: 'Drop / ignore',
};

const MATCH_LABELS: Record<string, string> = {
  contact: 'Email address',
  domain: 'Domain',
  keyword: 'Keyword',
  default: 'All emails (catch-all)',
};

interface MailRuleCardProps {
  agentId: string;
  output: string;
}

export function MailRuleCard({ agentId, output }: MailRuleCardProps) {
  const [state, setState] = useState<'pending' | 'approved' | 'discarded'>('pending');

  const isPolicy = output.includes('#!qorven:mail_policy_card');
  const payload = isPolicy ? parsePolicyCard(output) : parseRuleCard(output);

  const approve = async () => {
    if (isPolicy) {
      try {
        await agents.update(agentId, { mail_policy: payload.policy } as any);
        setState('approved');
        toast.success('Mail policy saved');
      } catch {
        toast.error('Failed to save policy');
      }
      return;
    }

    const ruleId = payload.rule_id;
    if (!ruleId) { toast.error('Missing rule ID'); return; }
    try {
      await confirmInboundRule(agentId, ruleId);
      setState('approved');
      toast.success('Rule activated');
    } catch {
      toast.error('Failed to confirm rule');
    }
  };

  const discard = async () => {
    if (!isPolicy) {
      const ruleId = payload.rule_id;
      if (ruleId) await discardInboundRule(agentId, ruleId).catch(() => {});
    }
    setState('discarded');
    toast.info(isPolicy ? 'Policy draft discarded' : 'Rule discarded');
  };

  if (state === 'approved') {
    return (
      <div className="my-2 flex items-center gap-2 rounded-xl border border-emerald-300/50 bg-emerald-50/50 px-4 py-3 text-sm dark:bg-emerald-900/20">
        <Check className="h-4 w-4 text-emerald-500" />
        <span className="text-emerald-700 dark:text-emerald-400">
          {isPolicy ? 'Mail policy saved.' : 'Rule is now active.'}
        </span>
      </div>
    );
  }

  if (state === 'discarded') {
    return (
      <div className="my-2 flex items-center gap-2 rounded-xl border border-border bg-muted/30 px-4 py-3 text-sm text-muted-foreground">
        <X className="h-4 w-4" />
        {isPolicy ? 'Policy draft discarded.' : 'Rule discarded.'}
      </div>
    );
  }

  return (
    <div className="my-2 rounded-xl border border-primary/30 bg-primary/5 overflow-hidden">
      <div className="flex items-center gap-2 border-b border-primary/20 px-4 py-2.5">
        <Mail className="h-4 w-4 text-primary" />
        <span className="text-sm font-semibold text-primary">
          {isPolicy ? '📋 New mail policy proposed' : '📬 New mail rule proposed'}
        </span>
      </div>

      <div className="px-4 py-3 space-y-2 text-sm">
        {isPolicy ? (
          <div>
            <p className="text-xs font-medium text-muted-foreground mb-1">Proposed policy</p>
            <p className="rounded-lg border border-border bg-background px-3 py-2 text-sm leading-relaxed">
              {payload.policy}
            </p>
          </div>
        ) : (
          <>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <p className="text-xs text-muted-foreground">Match</p>
                <p className="font-medium">{MATCH_LABELS[payload.match_type ?? ''] ?? payload.match_type}</p>
              </div>
              {payload.match_value && (
                <div>
                  <p className="text-xs text-muted-foreground">Value</p>
                  <p className="font-mono font-medium">{payload.match_value}</p>
                </div>
              )}
              <div>
                <p className="text-xs text-muted-foreground">Action</p>
                <p className="font-medium">{MODE_LABELS[payload.mode ?? ''] ?? payload.mode}</p>
              </div>
            </div>
            {payload.reason && (
              <div className="rounded-lg bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
                {payload.reason}
              </div>
            )}
          </>
        )}
      </div>

      <div className="flex gap-2 border-t border-primary/20 px-4 py-3">
        <button
          onClick={approve}
          className="flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Check className="h-3 w-3" /> Approve
        </button>
        <button
          onClick={discard}
          className="flex items-center gap-1.5 rounded-lg border border-destructive/50 px-3 py-1.5 text-xs font-medium text-destructive hover:bg-destructive/10"
        >
          <X className="h-3 w-3" /> Discard
        </button>
      </div>
    </div>
  );
}

// Detects if a tool output string should be rendered as a MailRuleCard.
export function isMailCard(output: unknown): boolean {
  if (typeof output !== 'string') return false;
  return output.includes('#!qorven:mail_rule_card') || output.includes('#!qorven:mail_policy_card');
}
