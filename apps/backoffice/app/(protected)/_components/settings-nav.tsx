'use client';

import { usePathname } from 'next/navigation';

import { NavLink } from '@/app/(protected)/_components/nav-link';

export interface SettingsNavItem {
  href: string;
  label: string;
  attention?: string;
}

interface SettingsNavProps {
  title: string;
  items: SettingsNavItem[];
}

// The settings sections; the context column and the narrow screens' card give it its heading.
export function SettingsNav({ title, items }: SettingsNavProps) {
  const pathname = usePathname();

  return (
    // Below lg the sections are a page, listed on a card like the content of every other page.
    <nav
      aria-label={title}
      className="flex flex-col px-3 pb-3 gap-y-1 max-lg:mx-6 max-lg:mb-10 max-lg:p-2 max-lg:bg-card max-lg:border max-lg:border-border max-lg:rounded-1.5xl max-lg:shadow-e2"
    >
      {items.map((item) => (
        <NavLink
          key={item.href}
          href={item.href}
          label={item.label}
          active={pathname === item.href}
          attention={item.attention}
        />
      ))}
    </nav>
  );
}
