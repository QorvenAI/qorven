// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import Link from 'next/link';

export default function Error500Page() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center space-y-4">
        <h1 className="text-7xl font-semibold text-destructive">500</h1>
        <h2 className="text-2xl font-semibold">Internal Error</h2>
        <p className="text-sm text-muted-foreground max-w-sm mx-auto">Something went wrong on our end. Please try again later.</p>
        <Link href="/" className="inline-flex h-10 items-center rounded-lg bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary/90 transition-colors">Back to Home</Link>
      </div>
    </div>
  );
}
