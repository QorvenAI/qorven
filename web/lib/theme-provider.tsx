'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { apiBase } from '@/lib/api-url';

// Color presets with HSL values for CSS vars
export const COLOR_PRESETS = [
  { name: 'Violet', value: 'oklch(0.541 0.281 293.009)', hex: '#8b5cf6', tw: 'bg-violet-500' },
  { name: 'Blue', value: 'oklch(0.546 0.245 262.881)', hex: '#3b82f6', tw: 'bg-blue-500' },
  { name: 'Emerald', value: 'oklch(0.696 0.17 162.48)', hex: '#10b981', tw: 'bg-emerald-500' },
  { name: 'Rose', value: 'oklch(0.645 0.246 16.439)', hex: '#f43f5e', tw: 'bg-rose-500' },
  { name: 'Amber', value: 'oklch(0.769 0.188 70.08)', hex: '#f59e0b', tw: 'bg-amber-500' },
  { name: 'Cyan', value: 'oklch(0.715 0.143 215.221)', hex: '#06b6d4', tw: 'bg-cyan-500' },
];

// Full-palette theme presets (defined in css/config.qorven.css as [data-theme="id"]).
// swatchBg / swatchPrimary are preview-only colors for the Settings swatches.
// `swatch` is the accent color shown as a solid square in the picker.
export const THEME_PRESETS = [
  { id: 'violet',   name: 'Violet',   swatch: '#7c3aed' }, // violet-600
  { id: 'slate',    name: 'Slate',    swatch: '#18181b' }, // zinc-900 (monochrome)
  { id: 'ocean',    name: 'Ocean',    swatch: '#2563eb' }, // blue-600
  { id: 'sand',     name: 'Sand',     swatch: '#d97706' }, // amber-600 (warm surfaces)
  { id: 'midnight', name: 'Midnight', swatch: '#4f46e5' }, // indigo-600 (true-black dark)
  { id: 'graphite', name: 'Graphite', swatch: '#0d9488' }, // teal-600 (neutral gray)
];

export const FONT_OPTIONS = [
  { name: 'System Default', value: 'system-ui, -apple-system, sans-serif' },
  { name: 'Inter', value: '"Inter", sans-serif' },
  { name: 'Geist', value: '"Geist", sans-serif' },
  { name: 'DM Sans', value: '"DM Sans", sans-serif' },
];

export type DateFormat = 'relative' | 'short' | 'long' | 'iso';

export interface ThemeSettings {
  primaryColor: string;    // hex
  primaryOklch: string;    // oklch for CSS var
  fontFamily: string; // ok — ThemeSettings type definition
  fontScale: number;       // 0.8 - 1.2
  borderRadius: number;    // 0 - 16 (px)
  density: 'compact' | 'default' | 'comfortable';
  dateFormat: DateFormat;
  timezone: string;        // IANA timezone, e.g. "Asia/Kolkata"
  themePreset: string;     // id of a THEME_PRESETS entry (full palette)
}

const DEFAULT_SETTINGS: ThemeSettings = {
  primaryColor: '',   // empty = no custom accent override; the theme preset's primary is used
  primaryOklch: '',
  fontFamily: 'system-ui, -apple-system, sans-serif', // ok — default theme value
  fontScale: 1,
  borderRadius: 10,
  density: 'default',
  dateFormat: 'relative',
  timezone: typeof Intl !== 'undefined' ? Intl.DateTimeFormat().resolvedOptions().timeZone : 'Asia/Kolkata',
  themePreset: 'violet',
};

const STORAGE_KEY = 'qorven-theme';

function getThemeToken() {
  if (typeof window === 'undefined') return '';
  return localStorage.getItem('qorven_token') || process.env.NEXT_PUBLIC_API_TOKEN || '';
}

function loadSettings(): ThemeSettings {
  if (typeof window === 'undefined') return DEFAULT_SETTINGS;
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return { ...DEFAULT_SETTINGS, ...JSON.parse(stored) };
  } catch {}
  return DEFAULT_SETTINGS;
}

function saveSettings(settings: ThemeSettings) {
  if (typeof window === 'undefined') return;
  // Save to localStorage (instant)
  localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
  // Save to backend DB (permanent, syncs across devices)
  fetch(`${apiBase()}/user/preferences`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${getThemeToken()}` },
    body: JSON.stringify(settings),
  }).catch(() => {}); // silent fail — localStorage is the fallback
}

async function loadFromBackend(): Promise<ThemeSettings | null> {
  try {
    const res = await fetch(`${apiBase()}/user/preferences`, {
      headers: { Authorization: `Bearer ${getThemeToken()}` },
    });
    if (!res.ok) return null;
    const data = await res.json();
    if (data && data.primaryColor) return { ...DEFAULT_SETTINGS, ...data };
  } catch {}
  return null;
}

function applyToDOM(settings: ThemeSettings) {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;

  // Theme preset (full palette) — the CSS [data-theme] cascade handles light/dark.
  root.dataset.theme = settings.themePreset || 'violet';

  // Primary color — only override inline when the user has CUSTOMIZED it away
  // from the default. Otherwise leave it unset so the selected theme preset's
  // own primary shows through (inline style would otherwise always win).
  const customizedPrimary = !!settings.primaryColor;
  if (customizedPrimary) {
    root.style.setProperty('--primary', settings.primaryOklch);
    root.style.setProperty('--ring', settings.primaryOklch);
    root.style.setProperty('--chart-1', settings.primaryOklch);
  } else {
    root.style.removeProperty('--primary');
    root.style.removeProperty('--ring');
    root.style.removeProperty('--chart-1');
  }

  // Font
  root.style.setProperty('--font-sans', settings.fontFamily);
  root.style.fontFamily = settings.fontFamily;

  // Font scale
  root.style.fontSize = `${settings.fontScale * 100}%`;

  // Border radius
  root.style.setProperty('--radius', `${settings.borderRadius / 16}rem`);

  // Density
  const densityScale = settings.density === 'compact' ? 0.85 : settings.density === 'comfortable' ? 1.15 : 1;
  root.style.setProperty('--density', String(densityScale));
}

interface ThemeContextType {
  settings: ThemeSettings;
  updateSettings: (partial: Partial<ThemeSettings>) => void;
  resetSettings: () => void;
}

const ThemeContext = createContext<ThemeContextType>({
  settings: DEFAULT_SETTINGS,
  updateSettings: () => {},
  resetSettings: () => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [settings, setSettings] = useState<ThemeSettings>(DEFAULT_SETTINGS);

  // Load on mount: localStorage first (instant), then backend (authoritative)
  useEffect(() => {
    const local = loadSettings();
    setSettings(local);
    applyToDOM(local);

    // Then fetch from backend — if different, update
    loadFromBackend().then(remote => {
      if (remote) {
        setSettings(remote);
        saveSettings(remote); // sync localStorage
        applyToDOM(remote);
      }
    });
  }, []);

  const updateSettings = (partial: Partial<ThemeSettings>) => {
    const updated = { ...settings, ...partial };
    setSettings(updated);
    saveSettings(updated);
    applyToDOM(updated);
  };

  const resetSettings = () => {
    setSettings(DEFAULT_SETTINGS);
    saveSettings(DEFAULT_SETTINGS);
    applyToDOM(DEFAULT_SETTINGS);
  };

  return (
    <ThemeContext.Provider value={{ settings, updateSettings, resetSettings }}>
      {children}
    </ThemeContext.Provider>
  );
}

export function useThemeSettings() {
  return useContext(ThemeContext);
}

export { DEFAULT_SETTINGS };
