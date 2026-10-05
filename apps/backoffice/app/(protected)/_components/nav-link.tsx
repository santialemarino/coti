import Link from 'next/link';

import { cn } from '@repo/ui/lib';
import { AttentionDot } from '@/components/attention-dot';

interface NavLinkProps {
  href: string;
  label: string;
  active: boolean;
  /* Names what needs attention behind this entry; the entry then carries a dot. */
  attention?: string;
  className?: string;
}

/*
 * One entry in a navigation list — the top bar's sections and the settings sections both render
 * this, so the two cannot drift apart.
 *
 * The fills are `surface-*` rather than `muted`: `muted` is within half a point of lightness of the
 * page wash, so a nav that ever sits on it would lose its hover, and `surface-*` clears white too.
 *
 * Hover and press stay in one family and one step apart — a press that jumps from a neutral hover to
 * a brand tint reads as a different control answering, not as the one under the finger.
 */
export function NavLink({ href, label, active, attention, className }: NavLinkProps) {
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
      {attention ? <AttentionDot label={attention} className="ml-auto" /> : null}
    </Link>
  );
}
