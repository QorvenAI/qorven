'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { useState, useEffect, useCallback } from 'react';
import { toast } from 'sonner';
import { Loader2, X, Plus } from 'lucide-react';
import { Card, Row, Input, Btn } from './primitives';
import { onboarding, type WebsiteProfile } from '@/lib/api-content';

function TagList({ tags, onChange, placeholder }: { tags: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const [input, setInput] = useState('');

  const add = () => {
    const val = input.trim();
    if (val && !tags.includes(val)) {
      onChange([...tags, val]);
    }
    setInput('');
  };

  const remove = (idx: number) => {
    onChange(tags.filter((_, i) => i !== idx));
  };

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-1.5">
        {tags.map((tag, i) => (
          <span key={i} className="inline-flex items-center gap-1 rounded-md bg-accent px-2 py-0.5 text-xs text-foreground border border-border">
            {tag}
            <button type="button" onClick={() => remove(i)} className="hover:text-destructive transition-colors">
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-2">
        <input
          type="text"
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); add(); } }}
          placeholder={placeholder}
          className="flex-1 rounded-lg border border-border bg-input px-3 py-1.5 text-sm outline-none placeholder:text-muted-foreground/40 focus:border-primary"
        />
        <button type="button" onClick={add} disabled={!input.trim()}
          className="inline-flex items-center gap-1 rounded-lg border border-border px-2.5 py-1.5 text-sm hover:bg-accent disabled:opacity-40 transition-colors">
          <Plus className="h-3.5 w-3.5" /> Add
        </button>
      </div>
    </div>
  );
}

type AnalysisPhase = 'idle' | 'crawling' | 'analyzing' | 'done' | 'error';

export function OnboardingSettings() {
  const [url, setUrl] = useState('');
  const [phase, setPhase] = useState<AnalysisPhase>('idle');
  const [profile, setProfile] = useState<WebsiteProfile | null>(null);
  const [saving, setSaving] = useState(false);
  const [loadingProfile, setLoadingProfile] = useState(true);

  useEffect(() => {
    onboarding.getProfile()
      .then(p => {
        if (p && p.url) {
          setProfile(p);
          setUrl(p.url);
          setPhase('done');
        }
      })
      .catch(() => {})
      .finally(() => setLoadingProfile(false));
  }, []);

  const analyze = useCallback(async () => {
    if (!url.trim()) {
      toast.error('Please enter a URL');
      return;
    }
    setPhase('crawling');
    try {
      // Simulate progress phases (the API handles it all, but we show staged progress)
      const timer = setTimeout(() => setPhase('analyzing'), 4000);
      const result = await onboarding.analyze(url.trim());
      clearTimeout(timer);
      setProfile(result);
      setPhase('done');
      toast.success('Website analyzed successfully');
    } catch (err: any) {
      setPhase('error');
      toast.error(err?.message || 'Analysis failed');
    }
  }, [url]);

  const save = async () => {
    if (!profile) return;
    setSaving(true);
    try {
      const updated = await onboarding.updateProfile(profile);
      setProfile(updated);
      toast.success('Profile saved');
    } catch (err: any) {
      toast.error(err?.message || 'Could not save profile');
    } finally {
      setSaving(false);
    }
  };

  const updateField = <K extends keyof WebsiteProfile>(key: K, value: WebsiteProfile[K]) => {
    setProfile(p => p ? { ...p, [key]: value } : p);
  };

  if (loadingProfile) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <Card id="onboarding" title="Website Profile" description="Analyze your product website to generate a brand profile for content creation.">
        <Row label="Product URL" hint="Enter your website or landing page URL">
          <div className="flex gap-2">
            <Input
              value={url}
              onChange={setUrl}
              placeholder="https://yourproduct.com"
              className="flex-1"
            />
            <Btn
              onClick={analyze}
              loading={phase === 'crawling' || phase === 'analyzing'}
              disabled={phase === 'crawling' || phase === 'analyzing'}
            >
              Analyze
            </Btn>
          </div>
        </Row>

        {(phase === 'crawling' || phase === 'analyzing') && (
          <div className="flex items-center gap-3 rounded-lg border border-border bg-accent/50 px-4 py-3">
            <Loader2 className="h-4 w-4 animate-spin text-primary" />
            <div className="text-sm">
              <span className="font-medium">
                {phase === 'crawling' ? 'Crawling website...' : 'Analyzing content...'}
              </span>
              <p className="text-xs text-muted-foreground mt-0.5">
                {phase === 'crawling'
                  ? 'Fetching pages and extracting content'
                  : 'Identifying brand voice, audience, and key themes'}
              </p>
            </div>
          </div>
        )}

        {phase === 'error' && (
          <div className="rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
            Analysis failed. Check the URL and try again.
          </div>
        )}
      </Card>

      {profile && phase === 'done' && (
        <Card id="brand-profile" title="Brand Profile" description="Edit the extracted profile. Changes here influence all generated content.">
          <Row label="Product Name">
            <Input
              value={profile.product_name}
              onChange={v => updateField('product_name', v)}
              placeholder="Product name"
            />
          </Row>

          <Row label="Tagline">
            <Input
              value={profile.tagline}
              onChange={v => updateField('tagline', v)}
              placeholder="Short tagline or description"
            />
          </Row>

          <Row label="Target Audience" hint="Who is this product for?">
            <textarea
              value={profile.audience}
              onChange={e => updateField('audience', e.target.value)}
              placeholder="Describe your target audience"
              rows={2}
              className="w-full rounded-lg border border-border bg-input px-3 py-2 text-sm outline-none placeholder:text-muted-foreground/40 focus:border-primary resize-none"
            />
          </Row>

          <Row label="Brand Voice" hint="Tone and style of communication">
            <textarea
              value={profile.brand_voice}
              onChange={e => updateField('brand_voice', e.target.value)}
              placeholder="e.g. Professional yet approachable, data-driven"
              rows={2}
              className="w-full rounded-lg border border-border bg-input px-3 py-2 text-sm outline-none placeholder:text-muted-foreground/40 focus:border-primary resize-none"
            />
          </Row>

          <Row label="Competitors">
            <TagList
              tags={profile.competitors ?? []}
              onChange={v => updateField('competitors', v)}
              placeholder="Add competitor name"
            />
          </Row>

          <Row label="Value Props">
            <TagList
              tags={profile.value_props ?? []}
              onChange={v => updateField('value_props', v)}
              placeholder="Add value proposition"
            />
          </Row>

          <Row label="Keywords">
            <TagList
              tags={profile.keywords ?? []}
              onChange={v => updateField('keywords', v)}
              placeholder="Add keyword"
            />
          </Row>

          <div className="flex justify-end pt-3 border-t border-border/70 -mx-6 px-6 -mb-5 mt-2 pb-1">
            <Btn onClick={save} loading={saving}>Save Profile</Btn>
          </div>
        </Card>
      )}
    </div>
  );
}
