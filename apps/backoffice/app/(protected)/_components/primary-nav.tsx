'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { NavLink } from '@/app/(protected)/_components/nav-link';
import { isOrdersPath, ROUTES } from '@/config/routes';

/*
 * The seller's main navigation, rendered on every protected screen. All four sections are links;
 * Reportes and Administración land on placeholder screens until their real ones exist.
 */
export function PrimaryNav() {
  const t = useTranslations('common');
  const pathname = usePathname();

  // A section stays active through its children, so the RFQ detail keeps "Pedidos" lit.
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
    <nav aria-label={t('nav.orders')} className="ml-2 flex items-center gap-x-2">
      {items.map((item) => (
        <NavLink key={item.href} href={item.href} label={item.label} active={item.active} />
      ))}
    </nav>
  );
}
