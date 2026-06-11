'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useEffect, useState } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { ArrowLeft, Loader2, Save, Settings, Globe } from 'lucide-react';
import { getApp, patchApp, publishApp } from '@/lib/api-apps';
import type { QorvenApp, SettingDef } from '@/lib/api-apps';
import { PageShell } from '@/components/layouts/page-shell';
import { cn } from '@/lib/utils';

export default function AppSettingsClient() {
  const router = useRouter();
  const params = useParams<{ slug: string }>();
  let slug = params.slug;
  if (typeof window !== 'undefined' && slug === '__app__') {
    slug = window.location.pathname.split('/apps/')[1]?.split('/')[0] ?? slug;
  }

  const [app, setApp] = useState<QorvenApp | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [publishing, setPublishing] = useState(false);

  async function handlePublishToggle() {
    if (!app) return;
    setPublishing(true);
    try {
      const next = !app.external_enabled;
      await publishApp(app.id, next);
      setApp({ ...app, external_enabled: next });
      toast.success(next ? 'Published externally — start a tunnel in Settings → Network to make it reachable' : 'Unpublished — external surface is now closed');
    } catch {
      toast.error('Failed to update publish state (admin only)');
    } finally {
      setPublishing(false);
    }
  }

  useEffect(() => {
    if (!slug || slug === '__app__') return;
    setLoading(true);
    // Fetch by slug — API uses /apps/{id} but we need to list to find by slug
    import('@/lib/api-apps').then(({ listApps }) =>
      listApps().then(data => {
        const found = data.apps.find(a => a.slug === slug);
        if (found) {
          setApp(found);
          // Pre-fill from existing config
          const initial: Record<string, string> = {};
          for (const def of found.settings_schema ?? []) {
            const val = found.config?.[def.key];
            initial[def.key] = val !== undefined ? String(val) : (def.default ?? '');
          }
          setValues(initial);
        }
        setLoading(false);
      }).catch(() => setLoading(false))
    );
  }, [slug]);

  async function handleSave() {
    if (!app) return;
    setSaving(true);
    try {
      // Merge settings values into config
      const newConfig = { ...(app.config ?? {}) };
      for (const def of app.settings_schema ?? []) {
        if (values[def.key] !== undefined && values[def.key] !== '') {
          newConfig[def.key] = def.type === 'number' ? Number(values[def.key])
            : def.type === 'boolean' ? values[def.key] === 'true'
            : values[def.key];
        } else if (def.type !== 'secret') {
          // Clear non-secret empty fields
          delete newConfig[def.key];
        }
      }
      const updated = await patchApp(app.id, { config: newConfig });
      setApp(updated);
      toast.success('Settings saved');
    } catch {
      toast.error('Failed to save settings');
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col h-full min-h-0">
        <div className="flex items-center gap-2 text-muted-foreground py-8 justify-center">
          <Loader2 className="h-4 w-4 animate-spin" />
        </div>
      </div>
    );
  }

  if (!app) {
    return (
      <div className="flex flex-col h-full min-h-0">
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="font-medium">App not found</p>
          <p className="text-sm text-muted-foreground mt-1">&ldquo;{slug}&rdquo;</p>
        </div>
      </div>
    );
  }

  const schema = app.settings_schema ?? [];

  return (
    <PageShell
      title={`${app.display_name} — Settings`}
      description={`Configure settings for ${app.display_name}`}
      contentClassName="px-0 py-0 sm:px-0"
      actions={
        <div className="flex items-center gap-2">
          <button
            onClick={() => router.push(`/apps/${app.slug}`)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
          >
            <ArrowLeft className="h-4 w-4" />
            Back
          </button>
          {schema.length > 0 && (
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-2 px-3 py-1.5 rounded-md bg-primary text-primary-foreground text-sm font-medium hover:bg-primary/90 transition-colors disabled:opacity-50"
            >
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
              Save
            </button>
          )}
        </div>
      }
    >
      <div className="flex-1 min-w-0 overflow-y-auto px-6 pb-6">
        <div className="max-w-2xl">
          {/* External publishing */}
          <div className="rounded-xl border border-border bg-card mb-6">
            <div className="flex items-start justify-between gap-4 px-5 py-4">
              <div className="min-w-0">
                <p className="text-sm font-semibold flex items-center gap-1.5"><Globe className="h-4 w-4 text-muted-foreground" /> Publish externally</p>
                <p className="text-xs text-muted-foreground mt-1 max-w-md">
                  Expose this app's public pages/tools on the internet (via a tunnel). Only manifest-declared public surfaces are reachable — your admin API stays private. Start a tunnel in Settings → Network → Internet exposure.
                </p>
              </div>
              <button
                onClick={handlePublishToggle}
                disabled={publishing}
                className={cn(
                  'shrink-0 flex items-center gap-2 px-3 py-1.5 rounded-md text-sm font-medium transition-colors disabled:opacity-50',
                  app.external_enabled ? 'border border-border text-muted-foreground hover:bg-accent' : 'bg-primary text-primary-foreground hover:bg-primary/90',
                )}
              >
                {publishing ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Globe className="h-3.5 w-3.5" />}
                {app.external_enabled ? 'Unpublish' : 'Publish'}
              </button>
            </div>
          </div>

          {schema.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-20 text-center">
              <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-muted mb-4">
                <Settings className="h-7 w-7 text-muted-foreground" />
              </div>
              <p className="font-medium">No configurable settings</p>
              <p className="text-sm text-muted-foreground mt-1">
                This app has no settings defined in its manifest.
              </p>
            </div>
          ) : (
            <div className="flex flex-col gap-5 pt-2">
              {schema.map(def => (
                <SettingField
                  key={def.key}
                  def={def}
                  value={values[def.key] ?? ''}
                  onChange={v => setValues(prev => ({ ...prev, [def.key]: v }))}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </PageShell>
  );
}

function SettingField({ def, value, onChange }: { def: SettingDef; value: string; onChange: (v: string) => void }) {
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-2">
        <label className="text-sm font-medium" htmlFor={`setting-${def.key}`}>
          {def.label}
          {def.required && <span className="text-destructive ml-0.5">*</span>}
        </label>
        {def.help_url && (
          <a href={def.help_url} target="_blank" rel="noopener noreferrer"
            className="text-xs text-primary hover:underline">
            Help ↗
          </a>
        )}
      </div>
      {def.description && (
        <p className="text-xs text-muted-foreground">{def.description}</p>
      )}
      {def.type === 'boolean' ? (
        <button
          onClick={() => onChange(value === 'true' ? 'false' : 'true')}
          className={cn(
            'relative inline-flex h-5 w-9 items-center rounded-full transition-colors self-start',
            value === 'true' ? 'bg-primary' : 'bg-input',
          )}
        >
          <span className={cn(
            'inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform',
            value === 'true' ? 'translate-x-4.5' : 'translate-x-0.5',
          )} />
        </button>
      ) : def.type === 'select' ? (
        <select
          id={`setting-${def.key}`}
          value={value}
          onChange={e => onChange(e.target.value)}
          className="qr-input max-w-sm"
        >
          <option value="">Select…</option>
          {def.options?.map(opt => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
      ) : (
        <input
          id={`setting-${def.key}`}
          type={def.type === 'secret' ? 'password' : def.type === 'number' ? 'number' : def.type === 'url' ? 'url' : 'text'}
          value={value}
          onChange={e => onChange(e.target.value)}
          placeholder={def.placeholder ?? (def.type === 'secret' ? '••••••••' : '')}
          className="qr-input max-w-sm"
          autoComplete={def.type === 'secret' ? 'new-password' : undefined}
        />
      )}
    </div>
  );
}
