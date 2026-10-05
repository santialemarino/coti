'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { BriefcaseIcon, ChartColumnIcon, InboxIcon, UsersIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { cn } from '@repo/ui/lib';
import { NavLink } from '@/app/(protected)/_components/nav-link';
import { isOrdersPath, ROUTES } from '@/config/routes';

interface PrimaryNavProps {
  // A row in the header from lg up; a tab bar along the bottom of the screen below it.
  layout?: 'bar' | 'tabs';
  className?: string;
}

/*
 * The seller's main navigation, rendered on every protected screen. All four sections are links;
 * Reportes and Administración land on placeholder screens until their real ones exist.
 */
export function PrimaryNav({ layout = 'bar', className }: PrimaryNavProps) {
  const t = useTranslations('common');
  const pathname = usePathname();

  // A section stays active through its children: a client's page keeps "Clientes" lit.
  const inSection = (href: string) => pathname === href || pathname.startsWith(`${href}/`);

  // Pedidos lands on the queue, the day's work; its table is the other view inside the section.
  const items = [
    { href: ROUTES.home, label: t('nav.orders'), icon: InboxIcon, active: isOrdersPath(pathname) },
    {
      href: ROUTES.clients,
      label: t('nav.clients'),
      icon: UsersIcon,
      active: inSection(ROUTES.clients),
    },
    {
      href: ROUTES.reports,
      label: t('nav.reports'),
      icon: ChartColumnIcon,
      active: inSection(ROUTES.reports),
    },
    {
      href: ROUTES.administration,
      label: t('nav.administration'),
      icon: BriefcaseIcon,
      active: inSection(ROUTES.administration),
    },
  ];

  if (layout === 'tabs') {
    return (
      <nav aria-label={t('nav.main')} className={cn('grid h-full grid-cols-4 px-2', className)}>
        {items.map(({ href, label, icon: Icon, active }) => (
          <Link
            key={href}
            href={href}
            aria-current={active ? 'page' : undefined}
            className="group/tab flex flex-col items-center justify-center gap-y-1 rounded-lg outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
          >
            {/* The pill carries the state, so the tab reads as chosen without relying on colour alone. */}
            <span
              className={cn(
                'flex h-8 w-14 items-center justify-center rounded-full transition-[color,background-color] duration-150 ease-out-soft',
                active
                  ? 'bg-accent text-accent-foreground group-hover/tab:bg-accent-strong group-active/tab:bg-accent-stronger'
                  : 'text-foreground-muted group-hover/tab:bg-surface-hover group-hover/tab:text-foreground group-active/tab:bg-surface-active',
              )}
            >
              <Icon aria-hidden="true" className="size-5" />
            </span>
            <span
              className={cn(
                'transition-colors duration-150 ease-out-soft',
                active
                  ? 'text-paragraph-mini-semibold text-foreground'
                  : 'text-paragraph-mini text-foreground-muted',
              )}
            >
              {label}
            </span>
          </Link>
        ))}
      </nav>
    );
  }

  return (
    <nav aria-label={t('nav.main')} className={cn('ml-2 flex items-center gap-x-2', className)}>
      {items.map((item) => (
        <NavLink key={item.href} href={item.href} label={item.label} active={item.active} />
      ))}
    </nav>
  );
}
