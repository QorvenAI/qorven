// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

// Routes that don't need authentication
const PUBLIC_PATHS = [
  '/login',
  '/setup',
  '/forgot-password',
  '/reset',
  '/api/',          // all Next.js API proxy routes
  '/_next',
  '/favicon.ico',
  '/logo/',
  '/__qorven_runtime',  // backend port discovery — must be reachable before login
  '/livez',             // health probes — used by reconnect-banner, no auth needed
  '/readyz',
];

export function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // Skip auth check for public paths and static assets
  if (PUBLIC_PATHS.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  // Only the cookie counts — NEXT_PUBLIC_API_TOKEN is a server-side dev key,
  // not proof that this browser session has logged in.
  const cookieToken = request.cookies.get('qorven_token')?.value;

  if (!cookieToken) {
    const loginUrl = new URL('/login', request.url);
    loginUrl.searchParams.set('next', pathname);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
}

export const config = {
  // Run on all routes except Next.js internals and static files
  matcher: [
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp)$).*)',
  ],
};
