'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import React from 'react';
import { useStore } from '@/store';
import { ErrorBoundary } from '@/components/error-boundary';
import { useAppSettingTabs } from '@/components/apps/app-registry-context';
import { request as apiRequest, getToken } from '@/lib/api-core';
import { ProfileSettings }       from '@/components/settings/sections/profile-settings';
import { AppearanceSettings }    from '@/components/settings/sections/appearance-settings';
import { ServicesSettings }      from '@/components/settings/sections/services-settings';
import { NotificationsSettings } from '@/components/settings/sections/notifications-settings';
import { ApiKeysSettings }       from '@/components/settings/sections/api-keys-settings';
import { WorkspaceSettings }     from '@/components/settings/sections/workspace-settings';
import { DeveloperSettings }     from '@/components/settings/sections/developer-settings';
import { SystemSettings }        from '@/components/settings/sections/system-settings';
import { NetworkSettings }       from '@/components/settings/sections/network-settings';
import { VoiceSettingsWrapper }  from '@/components/settings/sections/voice-settings-wrapper';

export default function SettingsPage() {
  const activeTab = useStore((s) => s.settingsTab);
  const appSettingTabs = useAppSettingTabs();

  return (
    <ErrorBoundary>
      <div>
        <div className="mb-6">
          <h1 className="text-lg font-semibold">Settings</h1>
          <p className="text-sm text-muted-foreground mt-0.5">Manage your account and workspace preferences</p>
        </div>

        {activeTab === 'profile'       && <ProfileSettings />}
        {activeTab === 'appearance'    && <AppearanceSettings />}
        {activeTab === 'services'      && <ServicesSettings />}
        {activeTab === 'voice'         && <VoiceSettingsWrapper />}
        {activeTab === 'notifications' && <NotificationsSettings />}
        {activeTab === 'api-keys'      && <ApiKeysSettings />}
        {activeTab === 'workspace'     && <WorkspaceSettings />}
        {activeTab === 'developer'     && <DeveloperSettings />}
        {activeTab === 'network'       && <NetworkSettings />}
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
      </div>
    </ErrorBoundary>
  );
}
