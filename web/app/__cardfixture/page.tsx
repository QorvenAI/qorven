'use client';

// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

// Card fixture page — renders any widget card from a ?data=<base64 JSON>
// URL parameter. Used exclusively by the Playwright e2e tests so we
// can assert on the rendered output of each card type without
// spinning up the backend + seeding data.
//
// The route path starts with "__" so it's clearly non-public and
// unlikely to clash with real user routes. In production we don't
// hide this page — there's nothing sensitive here — but it's tiny
// and sees no regular traffic, so the cost is negligible.
//
// Usage from a Playwright test:
//   const data = btoa(JSON.stringify({ type: 'weather', data: {...} }));
//   await page.goto(`/__cardfixture?data=${data}`);
//   await expect(page.locator('text=Tokyo')).toBeVisible();

import { useEffect, useState } from 'react';
import { WeatherWidget, CalcWidget } from '@/components/chat/widgets';
import type { MessagePart } from '@/types/parts';

export default function CardFixturePage() {
  const [parts, setParts] = useState<MessagePart[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const params = new URLSearchParams(window.location.search);
    const raw = params.get('data');
    if (!raw) {
      setError('missing ?data= param');
      return;
    }
    try {
      // Decode the base64-encoded JSON payload. Supports either a
      // single widget ({type, data}) or a full parts array.
      const json = atob(raw);
      const parsed = JSON.parse(json);
      if (Array.isArray(parsed)) {
        setParts(parsed);
      } else if (parsed.type && parsed.data !== undefined) {
        setParts([{ type: 'widget', widgetType: parsed.type, widgetData: parsed.data } as MessagePart]);
      } else {
        setError('unrecognised payload shape');
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  if (error) {
    return (
      <div className="p-8 text-destructive" data-testid="fixture-error">
        Error: {error}
      </div>
    );
  }
  if (!parts) {
    return (
      <div className="p-8 text-muted-foreground" data-testid="fixture-loading">
        loading fixture…
      </div>
    );
  }
  return (
    <div className="p-8" data-testid="fixture-container">
      {parts.map((p, i) => {
        if (p.type === 'widget' && p.widgetType === 'weather') {
          return <WeatherWidget key={i} data={p.widgetData} />;
        }
        if (p.type === 'widget' && p.widgetType === 'calc') {
          return <CalcWidget key={i} expression={p.widgetData?.expression} result={p.widgetData?.result} />;
        }
        if (p.type === 'text') {
          return <div key={i} className="text-sm">{p.content}</div>;
        }
        return <pre key={i} className="text-xs text-muted-foreground">{JSON.stringify(p, null, 2)}</pre>;
      })}
    </div>
  );
}
