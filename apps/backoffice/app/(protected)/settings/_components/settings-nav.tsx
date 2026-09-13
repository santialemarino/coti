'use client';

import { usePathname } from 'next/navigation';

import { NavLink } from '@/app/(protected)/_components/nav-link';

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
      {items.map((item) => (
        <NavLink
          key={item.href}
          href={item.href}
          label={item.label}
          active={pathname === item.href}
        />
      ))}
    </nav>
  );
}
