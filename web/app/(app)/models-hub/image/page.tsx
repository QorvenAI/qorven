'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { MediaTab } from '../media-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function ImagePage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Image Models" description="Image generation providers — DALL-E 3, Stability AI, FLUX and more" />
      <MediaTab kind="image" />
    </div>
  );
}
