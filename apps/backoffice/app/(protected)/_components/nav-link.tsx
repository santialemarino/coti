import Link from 'next/link';

import { cn } from '@repo/ui/lib';

interface NavLinkProps {
  href: string;
  label: string;
  active: boolean;
  className?: string;
}

/*
 * One entry in a navigation list — the top bar's sections and the settings rail both render this,
 * so the two cannot drift apart.
 *
 * The fills are `surface-*` rather than `muted` because a nav sits on two different grounds in this
 * app: the header's white bar and the settings page's wash. `muted` is within half a point of
 * lightness of the wash, so a hover painted with it disappears on exactly the screen that has the
 * most entries to scan.
 *
 * Hover and press stay in one family and one step apart — a press that jumps from a neutral hover to
 * a brand tint reads as a different control answering, not as the one under the finger.
 */
export function NavLink({ href, label, active, className }: NavLinkProps) {
  return (
    <Link
      href={href}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex items-center px-3 py-2 border border-transparent rounded-lg outline-none',
        'transition-[color,background-color,border-color,box-shadow] duration-150 ease-out-soft',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        active
          ? 'bg-accent text-paragraph-sm-medium text-accent-foreground hover:bg-accent-strong active:bg-accent-stronger'
          : 'text-paragraph-sm text-foreground-muted hover:bg-surface-hover hover:text-foreground active:bg-surface-active',
        className,
      )}
    >
      {label}
    </Link>
  );
}
