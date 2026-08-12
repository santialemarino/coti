'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { cn } from '@repo/ui/lib';
import { ROUTES } from '@/config/routes';

const ITEM_CLASSES =
  'flex items-center px-3 py-2 border border-transparent rounded-lg outline-none transition-[color,background-color,border-color,box-shadow] duration-150 ease-out-soft active:bg-accent-strong focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45';

/*
 * The seller's main navigation, rendered on every protected screen. All four sections are links;
 * Clientes, Reportes and Administración currently land on placeholder screens and are swapped for
 * the real ones the day their routes exist.
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
      {items.map((item) => {
        // A section stays active through its children, so the RFQ detail keeps "Pedidos" lit.
        const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
        return (
          <Link
            key={item.href}
            href={item.href}
            aria-current={active ? 'page' : undefined}
            className={cn(
              ITEM_CLASSES,
              active
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
