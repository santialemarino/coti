import { ArrowRightIcon, ClipboardListIcon, PlusIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Badge, Button, Card, EmptyState } from '@repo/ui/components';
import { CustomerFooter } from '@/components/customer-footer';
import { CustomerHeader } from '@/components/customer-header';
import { PortalLink as Link } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import { customerPageMetadata, getCustomerAuthOptions } from '@/lib/api/customer-auth-options';
import { getCustomerOrders } from '@/lib/api/customer-orders';
import { getCustomerProfile } from '@/lib/api/customer-profile';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getFormatters } from '@/lib/i18n/formatters-server';

export const generateMetadata = () => customerPageMetadata('title');

export default async function OrdersPage({
  searchParams,
}: {
  searchParams: Promise<{ page?: string }>;
}) {
  await requireCustomer(ROUTES.orders);
  const portal = await getCustomerAuthOptions();
  const query = await searchParams;
  const parsed = Number(query.page || 1);
  const page = Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 1;
  const [{ orders, hasMore }, t, fmt] = await Promise.all([
    getCustomerOrders(page),
    getTranslations('customerOrders'),
    getFormatters(),
  ]);
  const profile = orders.length === 0 ? await getCustomerProfile() : null;
  const needsContactProof =
    profile !== null &&
    (profile.ownershipUnproved || !profile.contacts.some((contact) => contact.verified));
  return (
    <>
      <CustomerHeader
        section="orders"
        supplierName={portal.supplierName}
        logoUrl={portal.logoUrl}
      />
      <main className="flex min-h-[calc(100dvh-4rem)] flex-col items-center px-4 py-8 sm:px-6 sm:py-12">
        <div className="flex w-full max-w-4xl flex-col gap-y-8">
          <header className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex flex-col gap-y-2">
              <h1 className="text-heading-3">{t('title')}</h1>
              <p className="text-paragraph-sm text-foreground-muted">{t('description')}</p>
            </div>
            <Button asChild>
              <Link href={ROUTES.newRequest}>
                <PlusIcon aria-hidden="true" />
                {t('newRequest')}
              </Link>
            </Button>
          </header>
          {orders.length === 0 ? (
            <Card className="px-6">
              <EmptyState
                icon={ClipboardListIcon}
                title={t(needsContactProof ? 'emptyUnverified' : 'emptyTitle')}
                description={t(
                  !needsContactProof
                    ? 'empty'
                    : profile?.ownershipUnproved
                      ? 'proveEarlierContact'
                      : 'verifyContactHint',
                )}
              >
                <Button asChild variant="outline">
                  <Link href={needsContactProof ? ROUTES.profile : ROUTES.newRequest}>
                    {t(needsContactProof ? 'verifyContacts' : 'newRequest')}
                  </Link>
                </Button>
              </EmptyState>
            </Card>
          ) : (
            <ul className="flex flex-col gap-y-3">
              {orders.map((order) => (
                <li key={order.id}>
                  <Link
                    href={ROUTES.order(order.id)}
                    className="group block rounded-1.5xl outline-none focus-visible:ring-3 focus-visible:ring-ring/45"
                  >
                    <Card
                      interactive
                      className="flex-row flex-wrap items-center justify-between px-5 gap-3 group-active:bg-sunken"
                    >
                      <div className="flex flex-col gap-y-1">
                        <span className="text-paragraph-sm-medium">{order.branchName}</span>
                        <span className="text-paragraph-xs text-foreground-muted">
                          {fmt.timestamp(order.receivedAt)}
                        </span>
                      </div>
                      <div className="flex items-center gap-x-3">
                        <Badge>{t(`statuses.${order.status}`)}</Badge>
                        <ArrowRightIcon
                          aria-hidden="true"
                          className="size-4 text-foreground-subtle"
                        />
                      </div>
                    </Card>
                  </Link>
                </li>
              ))}
            </ul>
          )}
          <nav className="flex justify-between" aria-label={t('pagination')}>
            {page > 1 ? (
              <Button variant="outline" asChild>
                <Link href={`${ROUTES.orders}?page=${page - 1}`}>{t('previous')}</Link>
              </Button>
            ) : (
              <span />
            )}
            {hasMore ? (
              <Button variant="outline" asChild>
                <Link href={`${ROUTES.orders}?page=${page + 1}`}>{t('next')}</Link>
              </Button>
            ) : null}
          </nav>
        </div>
      </main>
      <CustomerFooter />
    </>
  );
}
