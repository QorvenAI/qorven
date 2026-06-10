'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// SidebarNav — renders NAV_GROUPS as collapsible accordion sections.
// The group owning the active route auto-expands; expand/collapse state
// is managed by the accordion-menu primitive (it derives the open group
// from matchPath/selectedValue, then tracks toggles in its own state).

import { useCallback } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  AccordionMenu, AccordionMenuItem, AccordionMenuSub,
  AccordionMenuSubTrigger, AccordionMenuSubContent,
} from '@/components/ui/accordion-menu';
import { Badge } from '@/components/ui/badge';
import { NAV_GROUPS } from './nav-config';

export function SidebarNav() {
  const pathname = usePathname();

  // A nav item matches when the path equals the route or is a parent prefix.
  // '/' only matches exactly (otherwise it would match everything).
  const matchPath = useCallback(
    (path: string): boolean =>
      path === pathname || (path.length > 1 && !!pathname?.startsWith(path)),
    [pathname],
  );

  return (
    <AccordionMenu
      type="multiple"
      selectedValue={pathname ?? ''}
      matchPath={matchPath}
      className="space-y-1 px-2 py-2"
      classNames={{
        item: 'h-8 px-2.5 gap-2.5 text-2sm font-normal text-muted-foreground rounded-md hover:text-foreground hover:bg-accent data-[selected=true]:bg-accent data-[selected=true]:text-foreground',
        subTrigger:
          'px-2.5 text-xs font-medium uppercase tracking-wide text-muted-foreground/70 hover:text-foreground hover:bg-transparent',
        subContent: 'ps-0 pt-1',
      }}
    >
      {NAV_GROUPS.map((group) => (
        <AccordionMenuSub key={group.id} value={group.id}>
          <AccordionMenuSubTrigger>
            <span className="flex-1 text-left">{group.title}</span>
          </AccordionMenuSubTrigger>
          <AccordionMenuSubContent type="multiple" parentValue={group.id}>
            {group.children.map((item) => (
              <AccordionMenuItem key={item.path} value={item.path}>
                <Link href={item.path} className="flex items-center gap-2.5 w-full">
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className="flex-1 truncate">{item.title}</span>
                  {item.badge && (
                    <Badge size="sm" variant="secondary" appearance="light">
                      {item.badge}
                    </Badge>
                  )}
                </Link>
              </AccordionMenuItem>
            ))}
          </AccordionMenuSubContent>
        </AccordionMenuSub>
      ))}
    </AccordionMenu>
  );
}
