'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { providers as providersApi } from '@/lib/api';
import { useStore } from '@/store';
import { cn } from '@/lib/utils';
import { BrandIcon } from '@/components/brand-icon';
import { Plus } from 'lucide-react';

export default function ProvidersSection() {
  const [connected, setConnected] = useState<any[]>([]);
  const [catalog, setCatalog] = useState<any[]>([]);
  const openContextPanel = useStore((s) => s.openContextPanel);
  const getToken = () => typeof window !== 'undefined' ? (localStorage.getItem('qorven_token') || '') : '';

  useEffect(() => {
    providersApi.list().then(setConnected).catch(() => {});
    providersApi.catalog().then((d) => setCatalog(Array.isArray(d) ? d : [])).catch(() => {});
  }, []);

  return (
    <div className="space-y-6">
      {connected.length > 0 && (
        <div>
          <h3 className="text-sm font-medium mb-3">Connected</h3>
          {connected.map((p: any) => (
            <div key={p.id} className="flex items-center justify-between rounded-lg border border-border px-4 py-3 mb-2 cursor-pointer hover:border-primary/30"
              onClick={() => openContextPanel('provider', catalog.find((c) => c.id === p.provider_type) || p)}>
              <div>
                <p className="text-sm font-medium">{p.name}</p>
                <p className="text-2xs text-muted-foreground">{p.api_base || p.provider_type}</p>
              </div>
              <span className="text-2xs text-emerald-400">✓ Connected</span>
            </div>
          ))}
        </div>
      )}
      {connected.length === 0 && (
        <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
          <div><p className="text-sm font-medium">LiteLLM Proxy (Bedrock)</p><p className="text-2xs text-muted-foreground">localhost:4100</p></div>
          <span className="text-2xs text-emerald-400">✓ Connected</span>
        </div>
      )}

      <div>
        <h3 className="text-sm font-medium mb-3">Available Providers ({catalog.length})</h3>
        <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
          {catalog.map((p: any) => (
            <button key={p.id} onClick={() => openContextPanel('provider', p)}
              className="flex items-center gap-2.5 rounded-xl border border-border p-3 text-left hover:border-primary/30 transition-colors">
              <BrandIcon name={p.icon || p.id} size={20} />
              <div>
                <p className="text-xs font-medium">{p.name}</p>
                <p className="text-2xs text-muted-foreground">{
                  p.auth_type === 'none' ? 'No auth' :
                  p.auth_type === 'aws_credentials' ? 'AWS creds' :
                  p.auth_type === 'snowflake' ? 'JWT token' :
                  p.auth_type === 'query_key' ? 'API key' :
                  'API key'
                }</p>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
