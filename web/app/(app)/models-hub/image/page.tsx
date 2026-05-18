'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { MediaTab } from '../media-tab';

export default function ImagePage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Image Models</h1>
        <p className="text-sm text-muted-foreground mt-1">Image generation providers — DALL-E 3, Stability AI, FLUX and more</p>
      </div>
      <MediaTab kind="image" />
    </div>
  );
}
