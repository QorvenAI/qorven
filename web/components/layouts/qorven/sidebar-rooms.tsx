'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarRooms — pins the rooms/hubs list at the top of the sidebar (company
// room first, per Cycle B ordering). Caps its height so it never pushes the
// grouped menus off-screen; RoomsSidebar scrolls internally (it is h-full).

import { RoomsSidebar } from '@/components/sidebar/rooms-sidebar';

export function SidebarRooms() {
  return (
    <div className="shrink-0 h-[38vh] min-h-[140px] overflow-hidden border-b border-border">
      <RoomsSidebar />
    </div>
  );
}
