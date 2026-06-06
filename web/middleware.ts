// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).
//
// Auth guard middleware — runs at the Edge before any page renders.
// Redirects unauthenticated OR expired-token requests to /login immediately,
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

/**
 * Decode a JWT payload without verifying the signature.
 * Only used to check the `exp` claim — signature verification
 * is done on the backend for every API request.
 */
function jwtExpiry(token: string): number | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    // Base64url decode the payload (Edge runtime has atob)
    const payload = JSON.parse(atob(parts[1]!.replace(/-/g, '+').replace(/_/g, '/')));
    return typeof payload.exp === 'number' ? payload.exp : null;
  } catch {
    return null;
  }
}

function isTokenExpired(token: string): boolean {
  const exp = jwtExpiry(token);
  if (exp === null) return true; // malformed — treat as expired
  // 60-second buffer — avoids false redirects from clock drift or slow navigation
  return Date.now() / 1000 > exp - 60;
}

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (PUBLIC_PATHS.some((p) => pathname.startsWith(p))) {
    return NextResponse.next();
  }

  const cookieToken = request.cookies.get('qorven_token')?.value;

  // No token → redirect immediately
  if (!cookieToken) {
    const loginUrl = new URL('/login', request.url);
    loginUrl.searchParams.set('next', pathname);
    return NextResponse.redirect(loginUrl);
  }

  // Token exists but is expired → clear it and redirect
  if (isTokenExpired(cookieToken)) {
    const loginUrl = new URL('/login', request.url);
    loginUrl.searchParams.set('next', pathname);
    loginUrl.searchParams.set('reason', 'session_expired');
    const response = NextResponse.redirect(loginUrl);
    // Clear the stale cookie so middleware doesn't keep seeing it
    response.cookies.set('qorven_token', '', { path: '/', maxAge: 0 });
    return response;
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    '/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|onnx|wasm|lottie)$).*)',
  ],
};
