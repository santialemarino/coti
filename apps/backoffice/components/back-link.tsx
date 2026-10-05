import Link from 'next/link';
import { ArrowLeftIcon } from 'lucide-react';

import { cn } from '@repo/ui/lib';

interface BackLinkProps {
  href: string;
  label: string;
  className?: string;
}

// A screen's way back to the list it was opened from, on its own line and named.
export function BackLink({ href, label, className }: BackLinkProps) {
  return (
    <Link
      href={href}
      className={cn(
        'group/back flex w-fit items-center gap-x-1.5 rounded-sm outline-none text-paragraph-xs-medium text-foreground-muted transition-colors duration-200 ease-out-soft hover:text-foreground focus-visible:text-foreground',
        className,
      )}
    >
      <ArrowLeftIcon
        aria-hidden="true"
        className="size-3.5 group-focus-visible/back:animate-focus-bump-soft"
      />
      {label}
    </Link>
  );
}
