'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Admin-only endpoint; we render the card for everyone and let the POST's 403
// surface as a toast. Backend enforces name regex + 8 MiB cap + reserved-name list.

import { useEffect, useState } from 'react';
import { Loader2, Upload, Trash2, FileArchive } from 'lucide-react';
import { toast } from 'sonner';
import { wasmPlugins, type WasmPlugin } from '@/lib/api';
import { Card, Row, Input } from './primitives';

export function WasmPluginsCard() {
  const [plugins, setPlugins] = useState<WasmPlugin[]>([]);
  const [loading, setLoading] = useState(true);
  const [file, setFile] = useState<File | null>(null);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [parameters, setParameters] = useState('');
  const [uploading, setUploading] = useState(false);

  const refresh = async () => {
    setLoading(true);
    try {
      const res = await wasmPlugins.list();
      setPlugins(res.plugins ?? []);
    } catch {
      setPlugins([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { refresh(); }, []);

  const upload = async () => {
    if (!file || !name.trim()) return;
    setUploading(true);
    try {
      await wasmPlugins.upload({
        file,
        name: name.trim(),
        description: description.trim() || undefined,
        parameters: parameters.trim() || undefined,
      });
      toast.success(`Plugin "${name}" uploaded`);
      setFile(null);
      setName('');
      setDescription('');
      setParameters('');
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const remove = async (pluginName: string) => {
    if (!confirm(`Revoke plugin "${pluginName}"?`)) return;
    try {
      await wasmPlugins.delete(pluginName);
      toast.success(`Plugin "${pluginName}" revoked`);
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Revoke failed');
    }
  };

  const fileSize = (n: number) => {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  };

  return (
    <Card
      id="wasm_plugins"
      title="Wasm Plugins"
      description="Upload custom tools compiled to wasip1. Name must match ^[a-z][a-z0-9_]{0,62}$. Max 8 MiB. Admin only."
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-dashed border-border/80 bg-card/40 p-3 space-y-2">
          <Row label=".wasm file" hint="Compile with GOOS=wasip1 GOARCH=wasm or tinygo">
            <input
              type="file"
              accept=".wasm,application/wasm"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              className="block w-full text-xs file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-3 file:py-1 file:text-xs file:font-medium file:text-primary-foreground hover:file:bg-primary/90"
            />
          </Row>
          {file && (
            <p className="pl-2 text-2xs text-muted-foreground">
              <FileArchive className="mr-1 inline h-3 w-3" />
              {file.name} · {fileSize(file.size)}
            </p>
          )}
          <Row label="Name" hint="Tool name agents will invoke. Lowercase, digits, underscores.">
            <Input value={name} onChange={setName} placeholder="my_custom_tool" />
          </Row>
          <Row label="Description" hint="Shown in the agent's tool catalog.">
            <Input value={description} onChange={setDescription} placeholder="What this tool does" />
          </Row>
          <Row label="Parameters" hint="Optional JSON schema describing the tool's inputs.">
            <Input value={parameters} onChange={setParameters} placeholder='{"type":"object","properties":{...}}' />
          </Row>
          <div className="flex justify-end pt-1">
            <button
              onClick={upload}
              disabled={uploading || !file || !name.trim()}
              className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {uploading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
              Upload
            </button>
          </div>
        </div>

        <div>
          <p className="mb-2 text-2xs font-medium uppercase tracking-wider text-muted-foreground">
            Installed ({plugins.length})
          </p>
          {loading ? (
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          ) : plugins.length === 0 ? (
            <p className="text-2xs text-muted-foreground">No Wasm plugins uploaded yet.</p>
          ) : (
            <ul className="divide-y divide-border/60 rounded-lg border border-border bg-card">
              {plugins.map((p) => (
                <li key={p.id} className="flex items-start gap-3 p-3 text-xs">
                  <FileArchive className="mt-0.5 h-3.5 w-3.5 shrink-0 text-primary" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-mono font-medium">{p.name}</span>
                      <span className="text-muted-foreground">· {fileSize(p.size_bytes)}</span>
                    </div>
                    {p.description && <p className="mt-0.5 text-muted-foreground">{p.description}</p>}
                    <p className="mt-0.5 font-mono text-xs text-muted-foreground/70" title={p.sha256}>
                      sha256 {p.sha256.slice(0, 16)}… · uploaded {new Date(p.created_at).toLocaleDateString()}
                    </p>
                  </div>
                  <button
                    onClick={() => remove(p.name)}
                    title="Revoke"
                    className="flex h-6 w-6 items-center justify-center rounded-sm text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Card>
  );
}
