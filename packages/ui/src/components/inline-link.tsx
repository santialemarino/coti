import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';

import { cn } from '../lib/utils';

const TONES = {
  brand: 'text-primary hover:decoration-primary',
  muted: 'text-foreground-muted hover:text-foreground hover:decoration-foreground',
  danger: 'text-danger-foreground hover:decoration-danger-foreground',
} as const;

/*
 * Text that acts — an auth link, a "see all", a footer link, an inline replay action. Not a button:
 * a button is a rectangle you press, this is a word you follow.
 *
 * Hover animates the underline in by transitioning `text-decoration-color` from transparent, which
 * avoids the layout shift an appearing underline would cause. Focus is deliberately a *different*
 * signal from hover — the gentlest bump — so keyboard users can tell focus from mouse-over, and the
 * native outline is dropped in favour of it.
 *
 * `asChild` exists so a Next `<Link>` or a router link can wear this styling without the design
 * system depending on a framework.
 */
function InlineLink({
  className,
  tone = 'brand',
  asChild = false,
  ...props
}: React.ComponentProps<'a'> & { tone?: keyof typeof TONES; asChild?: boolean }) {
  const Comp = asChild ? Slot : 'a';

  return (
    <Comp
      data-slot="inline-link"
      className={cn(
        'inline-flex items-center gap-x-1 rounded-sm underline decoration-transparent underline-offset-2',
        'transition-colors duration-200 ease-out-soft',
        'outline-none focus-visible:animate-focus-bump-subtle',
        "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        'text-paragraph-sm-medium',
        TONES[tone],
        className,
      )}
      {...props}
    />
  );
}

export { InlineLink };
