// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useEffect, useMemo } from 'react';
import { useStore } from '@/store';
import type { SoulActivity } from '@/types';

/** Subscribe to a Soul's live run state — Trigger.dev useRealtimeRun pattern */
export function useSoulRun(agentId: string | undefined) {
  const soulState = useStore((s) => agentId ? s.soulStates[agentId] : undefined);
  const liveEvents = useStore((s) => s.liveEvents);
  const events = useMemo(
    () => liveEvents.filter((e) => e.agent_id === agentId).slice(0, 20),
    [liveEvents, agentId],
  );

  return {
    activity: soulState?.activity ?? ('idle' as SoulActivity),
    lastEvent: soulState?.lastEvent,
    tokensToday: soulState?.tokensToday ?? 0,
    recentEvents: events,
  };
}

/** Subscribe to streaming tokens for a message — Trigger.dev Realtime Streams pattern */
export function useStreamingTokens(msgId: string) {
  const content = useStore((s) => s.streamingTokens[msgId] ?? '');
  const clearStream = useStore((s) => s.clearStream);

  useEffect(() => {
    return () => clearStream(msgId);
  }, [msgId, clearStream]);

  return { content, isStreaming: content.length > 0 };
}
