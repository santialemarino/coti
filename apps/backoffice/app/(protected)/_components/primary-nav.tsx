'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { cn } from '@repo/ui/lib';
import { NavLink } from '@/app/(protected)/_components/nav-link';
import { isOrdersPath, ROUTES } from '@/config/routes';

interface PrimaryNavProps {
  // A row in the header from lg up; a list at the top of the menu sheet below it.
  layout?: 'bar' | 'stack';
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
    { href: ROUTES.home, label: t('nav.orders'), active: isOrdersPath(pathname) },
    { href: ROUTES.clients, label: t('nav.clients'), active: inSection(ROUTES.clients) },
    { href: ROUTES.reports, label: t('nav.reports'), active: inSection(ROUTES.reports) },
    {
      href: ROUTES.administration,
      label: t('nav.administration'),
      active: inSection(ROUTES.administration),
    },
  ];

  return (
    <nav
      aria-label={t('nav.main')}
      className={cn(
        layout === 'bar' ? 'ml-2 flex items-center gap-x-2' : 'flex flex-col px-3 gap-y-1',
        className,
      )}
    >
      {items.map((item) => (
        <NavLink key={item.href} href={item.href} label={item.label} active={item.active} />
      ))}
    </nav>
  );
}
