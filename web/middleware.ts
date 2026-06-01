// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// Auth guard middleware — runs at the Edge before any page renders.
// Redirects unauthenticated requests to /login immediately,
// preventing the flash of unauthorized content.
//
// NOTE: This file is only used by the Next.js dev server.
// The static export (QORVEN_STATIC=1) embeds into the Go binary and
// does not support Edge middleware — the Go gateway handles auth there.

import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

// Skip auth check for these paths
const PUBLIC_PATHS = [
  '/login',
  '/setup',
  '/forgot-password',
  '/reset',
  '/api/',
  '/_next',
  '/favicon.ico',
  '/logo/',
  '/vad/',
  '/icons/',
  '/__qorven_runtime',
  '/livez',
  '/readyz',
  '/app-assets/',
];

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (PUBLIC_PATHS.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const cookieToken = request.cookies.get('qorven_token')?.value;

  if (!cookieToken) {
    const loginUrl = new URL('/login', request.url);
    loginUrl.searchParams.set('next', pathname);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|onnx|wasm|lottie)$).*)',
  ],
};
