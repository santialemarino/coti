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

// The settings sections, listed in the context column under its "Configuración" heading.
export function SettingsNav({ title, items }: SettingsNavProps) {
  const pathname = usePathname();

  return (
    <nav aria-label={title} className="flex flex-col px-3 pb-3 gap-y-1">
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
