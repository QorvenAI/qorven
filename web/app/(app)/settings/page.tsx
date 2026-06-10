'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import React from 'react';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { PageShell } from '@/components/layouts/page-shell';
import { ErrorBoundary } from '@/components/error-boundary';
import { useAppSettingTabs } from '@/components/apps/app-registry-context';
import { request as apiRequest, getToken } from '@/lib/api-core';
import { User, Palette, Globe, Mic, Bell, Key, Building2, Code, Network, Monitor, Plug, Package } from 'lucide-react';
import { ProfileSettings }       from '@/components/settings/sections/profile-settings';
import { AppearanceSettings }    from '@/components/settings/sections/appearance-settings';
import { ServicesSettings }      from '@/components/settings/sections/services-settings';
import { NotificationsSettings } from '@/components/settings/sections/notifications-settings';
import { ApiKeysSettings }       from '@/components/settings/sections/api-keys-settings';
import { WorkspaceSettings }     from '@/components/settings/sections/workspace-settings';
import { DeveloperSettings }     from '@/components/settings/sections/developer-settings';
import { SystemSettings }        from '@/components/settings/sections/system-settings';
import { NetworkSettings }       from '@/components/settings/sections/network-settings';
import { IntegrationsSettings }  from '@/components/settings/sections/integrations-settings';
import { VoiceSettingsWrapper }  from '@/components/settings/sections/voice-settings-wrapper';

const SETTINGS_TABS = [
  { icon: User,      label: 'Profile',       tab: 'profile' },
  { icon: Palette,   label: 'Appearance',    tab: 'appearance' },
  { icon: Globe,     label: 'Services',      tab: 'services' },
  { icon: Mic,       label: 'Voice',         tab: 'voice' },
  { icon: Bell,      label: 'Notifications', tab: 'notifications' },
  { icon: Key,       label: 'API Keys',      tab: 'api-keys' },
  { icon: Building2, label: 'Workspace',     tab: 'workspace' },
  { icon: Code,      label: 'Developer',     tab: 'developer' },
  { icon: Network,   label: 'Network',       tab: 'network' },
  { icon: Plug,      label: 'Integrations',  tab: 'integrations' },
  { icon: Monitor,   label: 'System',        tab: 'system' },
];

export default function SettingsPage() {
  const activeTab = useStore((s) => s.settingsTab);
  const setSettingsTab = useStore((s) => s.setSettingsTab);
  const appSettingTabs = useAppSettingTabs();

  return (
    <ErrorBoundary>
      <PageShell
        title="Settings"
        description="Manage your account and workspace preferences"
        toolbar={
          <div className="flex items-center gap-1 overflow-x-auto">
            {SETTINGS_TABS.map((t) => (
              <button
                key={t.tab}
                onClick={() => setSettingsTab(t.tab)}
                className={cn(
                  'flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium whitespace-nowrap transition-colors',
                  activeTab === t.tab
                    ? 'bg-accent text-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent/50',
                )}
              >
                <t.icon className="h-3.5 w-3.5" />
                {t.label}
              </button>
            ))}
            {appSettingTabs.map((tab) => {
              const tabKey = `app-${tab.appId}-${tab.id}`;
              return (
                <button
                  key={tabKey}
                  onClick={() => setSettingsTab(tabKey)}
                  className={cn(
                    'flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium whitespace-nowrap transition-colors',
                    activeTab === tabKey
                      ? 'bg-accent text-foreground'
                      : 'text-muted-foreground hover:text-foreground hover:bg-accent/50',
                  )}
                >
                  <Package className="h-3.5 w-3.5" />
                  {tab.label}
                </button>
              );
            })}
          </div>
        }
      >
        {activeTab === 'profile'       && <ProfileSettings />}
        {activeTab === 'appearance'    && <AppearanceSettings />}
        {activeTab === 'services'      && <ServicesSettings />}
        {activeTab === 'voice'         && <VoiceSettingsWrapper />}
        {activeTab === 'notifications' && <NotificationsSettings />}
        {activeTab === 'api-keys'      && <ApiKeysSettings />}
        {activeTab === 'workspace'     && <WorkspaceSettings />}
        {activeTab === 'developer'     && <DeveloperSettings />}
        {activeTab === 'network'       && <NetworkSettings />}
        {activeTab === 'integrations'  && <IntegrationsSettings />}
        {activeTab === 'system'        && <SystemSettings />}
        {appSettingTabs.map((tab) =>
          activeTab === `app-${tab.appId}-${tab.id}` ? (
            <ErrorBoundary key={tab.id}>
              {React.createElement(tab.component, {
                React,
                request: (path: string, init?: RequestInit) => apiRequest(path, init),
                token: getToken(),
                appId: tab.appId,
              })}
            </ErrorBoundary>
          ) : null
        )}
      </PageShell>
    </ErrorBoundary>
  );
}
