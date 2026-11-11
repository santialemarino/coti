'use client';

import Image from 'next/image';
import { Building2Icon, ClipboardListIcon, LogOutIcon, UserRoundIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import { signOutCustomer } from '@/app/login/actions';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';

interface CustomerHeaderProps {
  section: 'orders' | 'profile';
  // The customer is on the corralón's portal, so its identity leads, not Coti's.
  supplierName: string;
  logoUrl?: string;
}

export function CustomerHeader({ section, supplierName, logoUrl }: CustomerHeaderProps) {
  const t = useTranslations('customerOrders');

  return (
    <header className="w-full bg-background border-b border-border">
      <div className="mx-auto flex min-h-16 w-full max-w-5xl flex-wrap items-center justify-between px-4 py-3 gap-3 sm:px-6">
        <Link
          href={ROUTES.orders}
          className="flex min-w-0 items-center gap-x-2.5 rounded-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
        >
          {logoUrl ? (
            <Image
              src={logoUrl}
              alt=""
              width={32}
              height={32}
              unoptimized
              className="size-8 shrink-0 rounded-md object-contain"
            />
          ) : (
            <span className="flex size-8 shrink-0 items-center justify-center bg-accent rounded-md text-primary">
              <Building2Icon className="size-4" aria-hidden="true" />
            </span>
          )}
          <span className="truncate text-paragraph-sm-medium text-foreground">{supplierName}</span>
        </Link>
        <nav aria-label={t('navigation')} className="flex items-center gap-x-1">
          <Button asChild variant={section === 'orders' ? 'secondary' : 'ghost'} size="sm">
            <Link href={ROUTES.orders} aria-current={section === 'orders' ? 'page' : undefined}>
              <ClipboardListIcon aria-hidden="true" />
              {t('title')}
            </Link>
          </Button>
          <Button asChild variant={section === 'profile' ? 'secondary' : 'ghost'} size="sm">
            <Link href={ROUTES.profile} aria-current={section === 'profile' ? 'page' : undefined}>
              <UserRoundIcon aria-hidden="true" />
              {t('profile')}
            </Link>
          </Button>
          <form action={signOutCustomer}>
            <Button type="submit" variant="ghost" size="icon-sm" aria-label={t('logout')}>
              <LogOutIcon aria-hidden="true" />
            </Button>
          </form>
        </nav>
      </div>
    </header>
  );
}
