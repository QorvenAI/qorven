'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { VoiceModelsTab } from '../voice-tab';
import { PageShell } from '@/components/layouts/page-shell';

export default function SttPage() {
  return (
    <PageShell title="Speech-to-Text" description="STT drivers and configured transcription providers">
      <VoiceModelsTab kind="stt" />
    </PageShell>
  );
}
