'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { MediaTab } from '../media-tab';

export default function VideoPage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Video Models</h1>
        <p className="text-sm text-muted-foreground mt-1">Video generation providers — Sora, Runway, Kling and more</p>
      </div>
      <MediaTab kind="video" />
    </div>
  );
}
