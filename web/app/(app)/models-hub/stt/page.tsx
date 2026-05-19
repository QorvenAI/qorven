'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { VoiceModelsTab } from '../voice-tab';
import { CanvasHeader } from '@/components/layouts/canvas-header';

export default function SttPage() {
  return (
    <div className="space-y-5">
      <CanvasHeader title="Speech-to-Text" description="STT drivers and configured transcription providers" />
      <VoiceModelsTab kind="stt" />
    </div>
  );
}
