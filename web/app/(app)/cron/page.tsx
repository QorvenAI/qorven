'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { redirect } from 'next/navigation';

export default function CronRedirect() {
  redirect('/schedule');
}
