'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRouter } from 'next/navigation';
import { BookOpen, Share2 } from 'lucide-react';
import { SidebarMenuItem, SidebarGroupTitle } from './sidebar-primitives';

export function KnowledgeSidebar() {
  const router = useRouter();
  return (
    <>
      <SidebarGroupTitle>Knowledge</SidebarGroupTitle>
      <ul className="flex flex-col gap-px px-2.5">
        <SidebarMenuItem icon={BookOpen} label="Memories" onClick={() => router.push('/memories')} />
        <SidebarMenuItem icon={Share2} label="Knowledge Graph" onClick={() => router.push('/knowledge-graph')} />
      </ul>
    </>
  );
}
