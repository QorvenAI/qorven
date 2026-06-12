'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { driveApi } from '@/lib/api-workspace';

export function MirrorDialog({ scope, onClose, onSaved }: {
  scope: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [providers, setProviders] = useState<{ id: string; name: string; connected: boolean }[]>([]);
  const [provider, setProvider] = useState('');
  const [folderId, setFolderId] = useState('');
  const [saving, setSaving] = useState(false);

  // 'all' isn't a real backend scope; default to company for the mirror.
  const mirrorScope = scope === 'all' || scope === 'shared' ? 'company' : scope;

  useEffect(() => {
    driveApi.remotes()
      .then((rs) => {
        const connected = rs.filter((r) => r.connected);
        setProviders(connected.map((r) => ({ id: r.id, name: r.name, connected: r.connected })));
        if (connected.length) setProvider(connected[0]!.id);
      })
      .catch(() => setProviders([]));
  }, []);

  const save = async () => {
    if (!provider) return;
    setSaving(true);
    try {
      await driveApi.createMirror({ scope: mirrorScope, provider, remote_folder_id: folderId });
      onSaved();
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-center justify-center bg-black/40" onClick={onClose}>
      <div role="dialog" className="w-[440px] rounded-xl border border-border bg-card p-5 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <h2 className="mb-1 text-base font-semibold">Mirror to cloud drive</h2>
        <p className="mb-4 text-xs text-muted-foreground">
          Files in the <span className="font-medium">{mirrorScope}</span> space will be copied out to your connected cloud drive.
        </p>
        {providers.length === 0 ? (
          <p className="mb-4 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
            No cloud drive connected. Connect one in Settings → Connections first.
          </p>
        ) : (
          <>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">Provider</label>
            <select value={provider} onChange={(e) => setProvider(e.target.value)} className="qr-select mb-3 w-full">
              {providers.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">Target folder ID (optional)</label>
            <input value={folderId} onChange={(e) => setFolderId(e.target.value)} placeholder="Leave blank for root" className="qr-input mb-4 w-full" />
          </>
        )}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="qr-btn qr-btn-ghost">Cancel</button>
          <button onClick={save} disabled={saving || !provider} className="qr-btn qr-btn-primary">{saving ? 'Saving…' : 'Create mirror'}</button>
        </div>
      </div>
    </div>
  );
}
