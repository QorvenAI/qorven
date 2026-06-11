'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { projectBriefs } from '@/lib/api-workspace';
import { RoomDetail } from '@/app/(app)/hubs/[id]/client';

interface Props {
  briefId: string;
}

export function ProjectHubView({ briefId }: Props) {
  const [roomId, setRoomId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    projectBriefs.hub(briefId)
      .then((r) => { if (!cancelled) setRoomId(r.room_id || null); })
      .catch(() => { /* non-fatal */ })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [briefId]);

  if (loading) {
    return (
      <div className="flex items-center justify-center gap-2 py-12">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        <span className="text-2sm text-muted-foreground">Setting up hub…</span>
      </div>
    );
  }

  if (!roomId) {
    return (
      <div className="flex items-center justify-center py-12">
        <p className="text-2sm text-muted-foreground">Hub not available for this project.</p>
      </div>
    );
  }

  return (
    <div className="h-full overflow-hidden">
      <RoomDetail roomId={roomId} showBack={false} />
    </div>
  );
}
