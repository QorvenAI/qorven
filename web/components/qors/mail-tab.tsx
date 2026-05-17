'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { ExternalLink } from 'lucide-react';
import Link from 'next/link';
import { MailPolicy } from './mail-policy';

// Mail settings (SMTP/IMAP setup) live in /mail — this tab shows only
// the inbound policy and rules so agents can be configured from chat.
export function MailTab({ agentId }: { agentId: string }) {
  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center border-b border-border px-4 py-2">
        <span className="text-xs font-medium">Inbound policy &amp; rules</span>
        <Link
          href="/mail"
          className="ml-auto flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
        >
          <ExternalLink className="h-3.5 w-3.5" />
          Open Inbox &amp; settings
        </Link>
      </div>
      <div className="flex-1 overflow-y-auto">
        <MailPolicy agentId={agentId} />
      </div>
    </div>
  );
}
