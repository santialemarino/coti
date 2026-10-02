'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { NavLink } from '@/app/(protected)/_components/nav-link';
import { ROUTES } from '@/config/routes';

/*
 * The seller's main navigation, rendered on every protected screen. All four sections are links;
 * Reportes and Administración land on placeholder screens until their real ones exist.
 */
export function PrimaryNav() {
  const t = useTranslations('common');
  const pathname = usePathname();

  const items = [
    { href: ROUTES.rfqs, label: t('nav.orders') },
    { href: ROUTES.clients, label: t('nav.clients') },
    { href: ROUTES.reports, label: t('nav.reports') },
    { href: ROUTES.administration, label: t('nav.administration') },
  ];

  return (
    <nav aria-label={t('nav.orders')} className="ml-2 flex items-center gap-x-2">
      {items.map((item) => (
        <NavLink
          key={item.href}
          href={item.href}
          label={item.label}
          /* A section stays active through its children, so the RFQ detail keeps "Pedidos" lit. */
          active={pathname === item.href || pathname.startsWith(`${item.href}/`)}
        />
      ))}
    </nav>
  );
}
