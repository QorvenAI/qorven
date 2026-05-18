// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { ExternalLink } from 'lucide-react';
import type { ChannelSchema } from './channel-schemas';

interface SetupGuideProps {
  schema: ChannelSchema;
}

export function SetupGuide({ schema }: SetupGuideProps) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
      <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide">Setup Guide</p>
      <ol className="space-y-2">
        {schema.setupSteps.map((step, i) => (
          <li key={i} className="flex gap-2.5 text-sm text-foreground">
            <span className="shrink-0 flex h-5 w-5 items-center justify-center rounded-full bg-primary/10 text-primary text-xs font-semibold">
              {i + 1}
            </span>
            <span>{step}</span>
          </li>
        ))}
      </ol>
      <div className="flex items-center gap-3 pt-1 border-t border-border">
        {schema.officialLink && (
          <a
            href={schema.officialLink}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-primary hover:underline flex items-center gap-1"
          >
            <ExternalLink className="h-3 w-3" />
            Create bot / token
          </a>
        )}
        <a
          href={`https://docs.qorven.ai/${schema.docsSlug}`}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs text-muted-foreground hover:text-foreground hover:underline flex items-center gap-1 ml-auto"
        >
          View full docs
          <ExternalLink className="h-3 w-3" />
        </a>
      </div>
    </div>
  );
}
