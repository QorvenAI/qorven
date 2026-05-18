// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import Link from 'next/link';

export default function Error404Page() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center space-y-4">
        <h1 className="text-7xl font-semibold text-primary">404</h1>
        <h2 className="text-2xl font-semibold">Page Not Found</h2>
        <p className="text-sm text-muted-foreground max-w-sm mx-auto">The page you&apos;re looking for doesn&apos;t exist or has been moved.</p>
        <Link href="/" className="inline-flex h-10 items-center rounded-lg bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors">Back to Home</Link>
      </div>
    </div>
  );
}
