import Image from 'next/image';

import { cn } from '@repo/ui/lib';
import isotype from '@/public/brand/isotype.png';
import lockup from '@/public/brand/lockup.png';
import wordmark from '@/public/brand/wordmark.png';

/*
 * The mark is raster art, so it is rendered through next/image with a static import: that carries the
 * intrinsic dimensions, which is what stops the header reflowing as the logo decodes. Each asset is
 * exported at 3× the largest height used here.
 */
const ASSETS = {
  wordmark: { src: wordmark, ratio: wordmark.width / wordmark.height },
  lockup: { src: lockup, ratio: lockup.width / lockup.height },
  isotype: { src: isotype, ratio: isotype.width / isotype.height },
} as const;

const HEIGHTS = { sm: 20, md: 28, lg: 44, xl: 72 } as const;

interface BrandProps {
  variant?: keyof typeof ASSETS;
  size?: keyof typeof HEIGHTS;
  /* Decorative next to a visible product name; otherwise it must carry one. */
  label?: string;
  className?: string;
}

export function Brand({ variant = 'wordmark', size = 'md', label, className }: BrandProps) {
  const { src, ratio } = ASSETS[variant];
  const height = HEIGHTS[size];

  return (
    <Image
      src={src}
      alt={label ?? ''}
      aria-hidden={label ? undefined : true}
      height={height}
      width={Math.round(height * ratio)}
      priority
      className={cn('select-none', className)}
    />
  );
}
