import Link from 'next/link';
import { getTranslations } from 'next-intl/server';

import { Badge, Button, Card } from '@repo/ui/components';
import { signOutCustomer } from '@/app/login/actions';
import { ROUTES } from '@/config/routes';
import { getCustomerOrders } from '@/lib/api/customer-orders';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getFormatters } from '@/lib/i18n/formatters-server';

export default async function OrdersPage({
  searchParams,
}: {
  searchParams: Promise<{ page?: string }>;
}) {
  await requireCustomer(ROUTES.orders);
  const query = await searchParams;
  const parsed = Number(query.page || 1);
  const page = Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 1;
  const [orders, t, fmt] = await Promise.all([
    getCustomerOrders(page),
    getTranslations('customerOrders'),
    getFormatters(),
  ]);
  return (
    <main className="flex min-h-dvh flex-col items-center px-4 py-8 sm:px-6 sm:py-12">
      <div className="flex w-full max-w-3xl flex-col gap-y-6">
        <header className="flex flex-wrap items-center justify-between gap-4">
          <h1 className="text-heading-4">{t('title')}</h1>
          <div className="flex items-center gap-x-2">
            <Button asChild>
              <Link href={ROUTES.newRequest}>{t('newRequest')}</Link>
            </Button>
            <form action={signOutCustomer}>
              <Button type="submit" variant="ghost">
                {t('logout')}
              </Button>
            </form>
          </div>
        </header>
        {orders.length === 0 ? (
          <Card className="px-6 text-paragraph-sm text-foreground-muted">{t('empty')}</Card>
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
                    <Badge>{t(`statuses.${order.status}`)}</Badge>
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
          {orders.length > 0 ? (
            <Button variant="outline" asChild>
              <Link href={`${ROUTES.orders}?page=${page + 1}`}>{t('next')}</Link>
            </Button>
          ) : null}
        </nav>
      </div>
    </main>
  );
}
