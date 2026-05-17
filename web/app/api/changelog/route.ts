// Copyright 2026 Qorven AI. Licensed under FSL-1.1-ALv2.

import { NextResponse } from 'next/server';
import { readFileSync } from 'fs';
import { join } from 'path';

export const dynamic = 'force-dynamic';

export function GET() {
  try {
    const p = join(process.cwd(), '..', 'CHANGELOG.md');
    const raw = readFileSync(p, 'utf-8');
    return NextResponse.json({ changelog: raw });
  } catch {
    return NextResponse.json({ changelog: '' }, { status: 500 });
  }
}
