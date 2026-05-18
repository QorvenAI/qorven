'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { VoiceModelsTab } from '../voice-tab';

export default function SttPage() {
  return (
    <div className="space-y-5">
      <div className="pb-2">
        <h1 className="text-lg font-semibold leading-none">Speech-to-Text</h1>
        <p className="text-sm text-muted-foreground mt-1">STT drivers and configured transcription providers</p>
      </div>
      <VoiceModelsTab kind="stt" />
    </div>
  );
}
