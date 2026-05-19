'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { MediaTab } from '../media-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function VideoPage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Video Models" description="Video generation providers — Sora, Runway, Kling and more" />
      <MediaTab kind="video" />
    </div>
  );
}
