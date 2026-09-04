'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';

import { cn } from '@repo/ui/lib';

export interface SettingsNavItem {
  href: string;
  label: string;
}

interface SettingsNavProps {
  title: string;
  items: SettingsNavItem[];
}

export function SettingsNav({ title, items }: SettingsNavProps) {
  const pathname = usePathname();

  return (
    <nav aria-label={title} className="flex flex-col shrink-0 gap-y-1 lg:w-56">
      <p className="px-3 pb-1 text-paragraph-xs-medium text-foreground-subtle uppercase">{title}</p>
      {items.map((item) => {
        const current = pathname === item.href;
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={current ? 'page' : undefined}
            className={cn(
              'flex items-center px-3 py-2 border border-transparent rounded-lg outline-none',
              'transition-[color,background-color,border-color,box-shadow] duration-150 ease-out-soft',
              'active:bg-accent-strong focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
              // The current entry keeps its highlight under the pointer; hovering it must not
              // wash it back to the resting treatment.
              current
                ? 'hover:bg-accent-strong bg-accent text-paragraph-sm-medium text-accent-foreground'
                : 'hover:bg-muted text-paragraph-sm text-foreground-muted hover:text-foreground',
            )}
          >
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
