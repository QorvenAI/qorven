'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { VoiceModelsTab } from '../voice-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function TtsPage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Text-to-Speech" description="TTS voice drivers and configured providers" />
      <VoiceModelsTab kind="tts" />
    </div>
  );
}
