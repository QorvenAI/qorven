'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { VoiceModelsTab } from '../voice-tab';

export default function TtsPage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Text-to-Speech</h1>
        <p className="text-sm text-muted-foreground mt-1">TTS voice drivers and configured providers</p>
      </div>
      <VoiceModelsTab kind="tts" />
    </div>
  );
}
