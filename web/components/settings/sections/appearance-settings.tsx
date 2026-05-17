'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { CheckCircle2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useThemeSettings, COLOR_PRESETS, FONT_OPTIONS, type DateFormat } from '@/lib/theme-provider';
import { Card, Row, Input } from './primitives';
import { formatDate } from '@/lib/format-date';

// IANA timezones — common selection
const TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Europe/Moscow',
  'Africa/Cairo',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Asia/Shanghai',
  'Australia/Sydney',
  'Pacific/Auckland',
];

const DATE_FORMAT_OPTIONS: { value: DateFormat; label: string }[] = [
  { value: 'relative', label: 'Relative (2 hours ago)' },
  { value: 'short',    label: 'Short (May 17, 2:30 PM)' },
  { value: 'long',     label: 'Long (May 17, 2026, 2:30 PM)' },
  { value: 'iso',      label: 'ISO 8601 (2026-05-17T14:30:00Z)' },
];

export function AppearanceSettings() {
  const { settings, updateSettings, resetSettings } = useThemeSettings();

  return (
    <div className="space-y-4">
      <Card id="app_color" title="Brand Color" description="The primary accent color used across buttons, highlights, and active states.">
        <Row label="Color Preset">
          <div className="space-y-3">
            <div className="flex items-center gap-2 flex-wrap">
              {COLOR_PRESETS.map(c => (
                <button key={c.name} onClick={() => updateSettings({ primaryColor: c.hex, primaryOklch: c.value })}
                  title={c.name}
                  className={cn('relative h-7 w-7 rounded-full border-2 transition-all cursor-pointer',
                    settings.primaryColor === c.hex ? 'border-foreground scale-110' : 'border-transparent hover:border-foreground/30')}
                  style={{ backgroundColor: c.hex }}>
                  {settings.primaryColor === c.hex && <CheckCircle2 className="absolute inset-0 m-auto h-3.5 w-3.5 text-white drop-shadow" />}
                </button>
              ))}
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground shrink-0">Custom:</span>
              <input type="color" value={settings.primaryColor}
                onChange={e => updateSettings({ primaryColor: e.target.value, primaryOklch: e.target.value })}
                className="h-8 w-10 rounded border border-border cursor-pointer p-0.5 bg-background" />
              <Input value={settings.primaryColor}
                onChange={v => updateSettings({ primaryColor: v, primaryOklch: v })}
                className="w-28 font-mono text-xs" />
            </div>
          </div>
        </Row>
      </Card>

      <Card id="app_typography" title="Typography & Layout" description="Adjust font, size, and spacing to match your preference.">
        <Row label="Font Family">
          <select value={settings.fontFamily} onChange={e => updateSettings({ fontFamily: e.target.value })} // ok — user font picker
            className="qr-select">
            {FONT_OPTIONS.map(f => <option key={f.name} value={f.value}>{f.name}</option>)}
          </select>
        </Row>
        <Row label="Font Size" hint={`Currently ${Math.round(settings.fontScale * 100)}% of default`}>
          <div className="space-y-1.5">
            <input type="range" min="0.8" max="1.2" step="0.05" value={settings.fontScale}
              onChange={e => updateSettings({ fontScale: parseFloat(e.target.value) })}
              className="w-full accent-primary" />
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>80% — Compact</span><span>100% — Default</span><span>120% — Large</span>
            </div>
          </div>
        </Row>
        <Row label="Border Radius" hint={`${settings.borderRadius}px`}>
          <div className="space-y-1.5">
            <input type="range" min="0" max="16" step="1" value={settings.borderRadius}
              onChange={e => updateSettings({ borderRadius: parseInt(e.target.value) })}
              className="w-full accent-primary" />
            <div className="flex justify-between text-xs text-muted-foreground">
              <span>Sharp</span><span>Rounded</span><span>Pill</span>
            </div>
          </div>
        </Row>
        <Row label="Density" hint="Controls padding and spacing throughout the UI">
          <div className="flex gap-2">
            {(['compact', 'default', 'comfortable'] as const).map(d => (
              <button key={d} onClick={() => updateSettings({ density: d })}
                className={cn('rounded-lg border px-4 py-2 text-xs font-medium capitalize cursor-pointer transition-colors',
                  settings.density === d ? 'border-primary bg-primary/10 text-primary' : 'border-border text-muted-foreground hover:text-foreground hover:bg-accent')}>
                {d}
              </button>
            ))}
          </div>
        </Row>
        <div className="flex justify-between items-center pt-3 border-t border-border/70 -mx-6 px-6 -mb-5 pb-1">
          <button onClick={resetSettings} className="text-xs text-muted-foreground hover:text-foreground cursor-pointer transition-colors">
            Reset all to defaults
          </button>
          <span className="text-xs text-muted-foreground">All appearance changes are instant</span>
        </div>
      </Card>

      <Card id="app_datetime" title="Date & Time" description="Control how dates and times are displayed throughout Qorven.">
        <Row label="Date Format">
          <div className="space-y-2">
            <div className="flex flex-col gap-1.5">
              {DATE_FORMAT_OPTIONS.map(opt => (
                <label key={opt.value} className="flex items-center gap-2.5 cursor-pointer">
                  <input
                    type="radio"
                    name="dateFormat"
                    value={opt.value}
                    checked={settings.dateFormat === opt.value}
                    onChange={() => updateSettings({ dateFormat: opt.value })}
                    className="accent-primary"
                  />
                  <span className="text-sm">{opt.label}</span>
                </label>
              ))}
            </div>
            <p className="text-xs text-muted-foreground pt-1">
              Preview: <span className="text-foreground font-medium">{formatDate(new Date(), settings.dateFormat, settings.timezone)}</span>
            </p>
          </div>
        </Row>
        <Row label="Timezone">
          <div className="space-y-1.5">
            <select
              value={settings.timezone}
              onChange={e => updateSettings({ timezone: e.target.value })}
              className="qr-select"
            >
              {TIMEZONES.map(tz => (
                <option key={tz} value={tz}>{tz.replace(/_/g, ' ')}</option>
              ))}
            </select>
            <p className="text-xs text-muted-foreground">
              Current time in zone: <span className="text-foreground font-medium">
                {new Date().toLocaleTimeString(undefined, { timeZone: settings.timezone, hour: '2-digit', minute: '2-digit', timeZoneName: 'short' })}
              </span>
            </p>
          </div>
        </Row>
      </Card>

      <Card id="app_preview" title="Live Preview">
        <div className="grid sm:grid-cols-2 gap-4">
          <div className="space-y-2.5">
            <div className="rounded-xl border border-border p-4 space-y-2.5">
              <div className="flex items-center gap-2.5">
                <div className="h-8 w-8 rounded-full bg-primary flex items-center justify-center text-xs font-bold text-primary-foreground">Q</div>
                <div>
                  <p className="text-sm font-medium">Qor Agent</p>
                  <p className="text-xs text-muted-foreground">Just now</p>
                </div>
              </div>
              <p className="text-sm leading-relaxed">Here's a sample agent message with your current theme applied across all UI elements.</p>
              <div className="flex gap-1.5 flex-wrap">
                {['Source 1', 'Source 2', 'Source 3'].map(s => (
                  <span key={s} className="rounded-full border border-border bg-muted/30 px-2 py-0.5 text-xs">{s}</span>
                ))}
              </div>
            </div>
          </div>
          <div className="space-y-2">
            <button className="w-full rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground">Primary Action</button>
            <input placeholder="Input field…" readOnly className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground/40 outline-none" />
            <div className="rounded-xl border border-border p-3 flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">Card component</p>
                <p className="text-xs text-muted-foreground">Supporting description text</p>
              </div>
              <div className="flex gap-1.5">
                <span className="rounded-full bg-primary/10 text-primary px-2 py-0.5 text-xs font-medium">Active</span>
                <span className="rounded-full bg-emerald-400/10 text-emerald-400 px-2 py-0.5 text-xs font-medium">Live</span>
              </div>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}
