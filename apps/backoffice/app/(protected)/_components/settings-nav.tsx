'use client';

import { usePathname } from 'next/navigation';

import { Separator } from '@repo/ui/components';
import { NavLink } from '@/app/(protected)/_components/nav-link';
import { ROUTES } from '@/config/routes';
import { useNavigationPending } from '@/hooks/use-navigation-pending';

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
  // The destination is marked on the click, before its page arrives.
  const committed = usePathname();
  const pathname = useNavigationPending().pendingPathname ?? committed;

  return (
    // Below lg the sections are a page, listed on a card like the content of every other page.
    <nav
      aria-label={title}
      className="flex flex-col px-3 pb-3 gap-y-1 max-lg:mx-6 max-lg:mb-10 max-lg:p-2 max-lg:bg-card max-lg:border max-lg:border-border max-lg:rounded-1.5xl max-lg:shadow-e2"
    >
      {items.map((item) => (
        <div key={item.href} className="flex flex-col gap-y-1">
          {item.href === ROUTES.onboardingSettings ? <Separator className="my-2" /> : null}
          <NavLink
            href={item.href}
            label={item.label}
            active={pathname === item.href || pathname.startsWith(`${item.href}/`)}
            attention={item.attention}
          />
        </div>
      ))}
    </nav>
  );
}
