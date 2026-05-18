'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useServiceEnabled } from './use-service-enabled';

export function useVoiceEnabled(): { enabled: boolean; loading: boolean } {
  return useServiceEnabled('services.voice');
}
