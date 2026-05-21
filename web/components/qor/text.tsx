'use client';

// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

import * as React from 'react';
import { cn } from '@/lib/utils';
import { cva, type VariantProps } from 'class-variance-authority';

// Qorven type scale — 5 sizes × 5 color variants.
// Change size or color here and every Text/Heading/Caption/Hint instance updates.
const textVariants = cva('', {
  variants: {
    size: {
      '2xs':  'text-2xs',
      xs:     'text-xs',
      '2sm':  'text-2sm',
      sm:     'text-sm',
      base:   'text-base',
    },
    variant: {
      default:     'text-foreground',
      muted:       'text-muted-foreground',
      primary:     'text-primary',
      destructive: 'text-destructive',
      success:     'text-[var(--color-success-accent,var(--color-green-600))]',
    },
    weight: {
      normal:   'font-normal',
      medium:   'font-medium',
      semibold: 'font-semibold',
      bold:     'font-bold',
    },
    truncate: {
      true: 'truncate',
    },
  },
  defaultVariants: {
    size:    'sm',
    variant: 'default',
    weight:  'normal',
  },
});

type TextElement = 'p' | 'span' | 'div' | 'label' | 'strong' | 'em';

interface TextProps
  extends React.HTMLAttributes<HTMLElement>,
    VariantProps<typeof textVariants> {
  as?: TextElement;
}

function Text({ as: Tag = 'span', size, variant, weight, truncate, className, ...props }: TextProps) {
  return (
    <Tag
      data-slot="text"
      className={cn(textVariants({ size, variant, weight, truncate }), className)}
      {...props}
    />
  );
}

// Shorthand aliases matching the Metronic naming pattern used in the codebase.
// Each is a Text with opinionated defaults so call-sites stay concise.

function Caption({ className, ...props }: Omit<TextProps, 'size'>) {
  return <Text size="xs" variant="muted" data-slot="caption" className={className} {...props} />;
}

function Hint({ className, ...props }: Omit<TextProps, 'size' | 'variant'>) {
  return <Text size="2xs" variant="muted" data-slot="hint" className={className} {...props} />;
}

function Heading({ className, ...props }: Omit<TextProps, 'size' | 'weight'>) {
  return <Text as="p" size="sm" weight="semibold" data-slot="heading" className={className} {...props} />;
}

export { Text, Caption, Hint, Heading, textVariants };
export type { TextProps };
