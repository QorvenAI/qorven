'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { MediaTab } from '../media-tab';
import { PageShell } from '@/components/layouts/page-shell';

export default function VideoPage() {
  return (
    <PageShell title="Video Models" description="Video generation providers — Sora, Runway, Kling and more">
      <MediaTab kind="video" />
    </PageShell>
  );
}
