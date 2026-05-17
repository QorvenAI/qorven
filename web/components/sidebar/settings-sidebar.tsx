'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { useRouter } from 'next/navigation';
import { useStore } from '@/store';
import { User, Palette, Globe, Mic, Bell, Key, Building2, Code, Network, Monitor, Package } from 'lucide-react';
import { SidebarMenuItem, SidebarDivider, SidebarGroupTitle } from './sidebar-primitives';
import { useAppSettingTabs } from '@/components/apps/app-registry-context';

export function SettingsSidebar() {
  const router = useRouter();
  const settingsTab = useStore((s) => s.settingsTab);
  const setSettingsTab = useStore((s) => s.setSettingsTab);
  const appSettingTabs = useAppSettingTabs();

  const items = [
    { icon: User,      label: 'Profile',       tab: 'profile' },
    { icon: Palette,   label: 'Appearance',    tab: 'appearance' },
    { icon: Globe,     label: 'Services',      tab: 'services' },
    { icon: Mic,       label: 'Voice',         tab: 'voice' },
    { icon: Bell,      label: 'Notifications', tab: 'notifications' },
    { icon: Key,       label: 'API Keys',      tab: 'api-keys' },
    { icon: Building2, label: 'Workspace',     tab: 'workspace' },
    { icon: Code,      label: 'Developer',     tab: 'developer' },
    { icon: Network,   label: 'Network',       tab: 'network' },
    { icon: Monitor,   label: 'System',        tab: 'system' },
  ];

  const go = (tab: string) => {
    setSettingsTab(tab);
    router.push('/settings');
  };

  return (
    <>
      <ul className="flex flex-col gap-px px-2.5 pt-3">
        {items.map((item) => (
          <SidebarMenuItem
            key={item.tab}
            icon={item.icon}
            label={item.label}
            active={settingsTab === item.tab}
            onClick={() => go(item.tab)}
          />
        ))}
      </ul>
      {appSettingTabs.length > 0 && (
        <>
          <SidebarDivider />
          <SidebarGroupTitle>App Settings ({appSettingTabs.length})</SidebarGroupTitle>
          <ul className="flex flex-col gap-px px-2.5">
            {appSettingTabs.map((tab) => {
              const tabKey = `app-${tab.appId}-${tab.id}`;
              return (
                <SidebarMenuItem
                  key={tabKey}
                  icon={Package}
                  label={tab.label}
                  active={settingsTab === tabKey}
                  onClick={() => go(tabKey)}
                />
              );
            })}
          </ul>
        </>
      )}
    </>
  );
}
