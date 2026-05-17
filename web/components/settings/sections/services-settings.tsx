'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { Globe, Bell, Code, Palette, Shield, Monitor, Zap, ArrowRight } from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import { useRouter } from 'next/navigation';
import { Card, Toggle, usePrefs } from './primitives';
import { notifyPrefsChange } from '@/hooks/use-service-enabled';

const SERVICE_GROUPS = [
  { key: 'services.web_search',            label: 'Web Search',            desc: 'Real-time search grounding for all agents',                                                                                                                   icon: Globe },
  { key: 'services.voice',                 label: 'Voice (TTS/STT)',        desc: 'Speech synthesis and recognition capabilities',                                                                                                             icon: Bell },
  { key: 'services.embeddings',            label: 'Embeddings',             desc: 'Document indexing for RAG knowledge pipelines',                                                                                                             icon: Code },
  { key: 'services.media',                 label: 'Media & Images',         desc: 'Image generation and media processing tools',                                                                                                               icon: Palette },
  { key: 'services.pii_redaction',         label: 'PII Redaction',          desc: 'Mask emails, phones, cards, SSNs in user messages + tool output before the model sees them',                                                              icon: Shield },
  { key: 'services.storage_write_enabled', label: 'Cloud Storage Writes',   desc: 'Allow storage_write / storage_copy / storage_sync to modify remote cloud storage. Read operations are always allowed.',                                   icon: Code },
  { key: 'services.screen_share',          label: 'Screen Share',           desc: "Let the agent see your screen when you click \"Share Screen\". Frames are held in memory for 30s only — never recorded.",                               icon: Monitor },
  { key: 'services.browser_live_view',     label: 'Agent Live View',        desc: "Show the agent’s Chromium browser as a live preview. Useful during demos; adds a few KB/s while running.",                                          icon: Monitor },
];

export function ServicesSettings() {
  const { prefs, savePrefs } = usePrefs();
  const router = useRouter();

  const toggle = async (key: string) => {
    const val = !prefs[key];
    try {
      await savePrefs({ [key]: val });
      notifyPrefsChange({ [key]: val });
      toast.success(`${val ? 'Enabled' : 'Disabled'} — changes apply to new agent runs`);
    } catch { toast.error('Could not save changes. Please try again.'); }
  };

  return (
    <div className="space-y-4">
      <Card id="global_services" title="Global Services"
        description="When enabled globally, all agents can use these capabilities. Disable per-agent in Qor settings to restrict individual agents.">
        <div className="space-y-2.5">
          {SERVICE_GROUPS.map(({ key, label, desc, icon: Icon }) => {
            const enabled = !!prefs[key];
            const hubLink =
              key === 'services.voice'      ? '/models-hub/tts' :
              key === 'services.web_search' ? '/models-hub/search' : null;
            return (
              <div key={key} className="flex items-center justify-between rounded-xl border border-border px-4 py-3 gap-3">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <div className={cn('flex h-9 w-9 items-center justify-center rounded-lg shrink-0 transition-colors',
                    enabled ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground')}>
                    <Icon className="h-4 w-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-medium">{label}</p>
                      {hubLink && enabled && (
                        <button onClick={() => router.push(hubLink)}
                          className="flex items-center gap-0.5 text-2xs text-muted-foreground hover:text-primary transition-colors">
                          Configure <ArrowRight className="h-2.5 w-2.5" />
                        </button>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground truncate">{desc}</p>
                  </div>
                </div>
                <Toggle checked={enabled} onChange={() => toggle(key)} />
              </div>
            );
          })}
        </div>
      </Card>

      <Card id="prompt_guard" title="Prompt-Injection Defense"
        description="Scan every user message for jailbreak and injection attempts. Off by default; Block is recommended for production deployments.">
        <div className="space-y-2">
          {[
            { value: 'off',    label: 'Off',    desc: 'No scanning. Lowest overhead, no protection.' },
            { value: 'warn',   label: 'Warn',   desc: 'Scan and log every detection; let all messages through. Good for pilots where you want to observe false-positive rates first.' },
            { value: 'block',  label: 'Block',  desc: 'Refuse messages that score ≥ 0.6 (Likely). Recommended default.' },
            { value: 'strict', label: 'Strict', desc: 'Refuse messages that score ≥ 0.3 (Suspicious). Higher false-positive rate; use only when downstream tools are especially sensitive.' },
          ].map(opt => {
            const active = (prefs['services.prompt_guard'] ?? 'off') === opt.value;
            return (
              <label key={opt.value}
                className={cn('flex items-start gap-3 rounded-xl border px-4 py-3 cursor-pointer transition-colors',
                  active ? 'border-primary bg-primary/5' : 'border-border hover:bg-accent/30')}>
                <input
                  type="radio"
                  name="prompt_guard"
                  value={opt.value}
                  checked={active}
                  onChange={async () => {
                    try {
                      await savePrefs({ 'services.prompt_guard': opt.value });
                      toast.success(`Prompt-injection defense: ${opt.label}`);
                    } catch { toast.error('Could not save changes. Please try again.'); }
                  }}
                  className="mt-0.5"
                />
                <div className="min-w-0">
                  <p className="text-sm font-medium">{opt.label}</p>
                  <p className="text-xs text-muted-foreground">{opt.desc}</p>
                </div>
              </label>
            );
          })}
        </div>
      </Card>

      <Card id="providers_link" title="LLM Providers" description="Manage connected AI providers and model selection in Models Hub.">
        <div className="flex items-center justify-between rounded-xl border border-border px-4 py-4">
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary shrink-0">
              <Zap className="h-4 w-4" />
            </div>
            <div>
              <p className="text-sm font-medium">Models Hub</p>
              <p className="text-xs text-muted-foreground">Connect providers, select models, configure routing</p>
            </div>
          </div>
          <button onClick={() => router.push('/models-hub')}
            className="flex items-center gap-1.5 rounded-lg border border-border px-3 py-1.5 text-xs font-medium hover:bg-accent transition-colors cursor-pointer">
            Open <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
      </Card>
    </div>
  );
}
