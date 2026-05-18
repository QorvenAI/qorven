// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import { type ClassValue, clsx } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
