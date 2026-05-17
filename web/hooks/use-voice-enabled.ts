'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useServiceEnabled } from './use-service-enabled';

export function useVoiceEnabled(): { enabled: boolean; loading: boolean } {
  return useServiceEnabled('services.voice');
}
