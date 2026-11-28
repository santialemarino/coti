import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { Button, Card } from '@repo/ui/components';
import { PublicQuoteView } from '@/app/quotes/_components/public-quote-view';
import { CustomerFooter } from '@/components/customer-footer';
import { CustomerHeader } from '@/components/customer-header';
import { PortalLink } from '@/components/portal-link';
import { ROUTES } from '@/config/routes';
import { customerPageMetadata, getCustomerAuthOptions } from '@/lib/api/customer-auth-options';
import { getCustomerOrder, getCustomerOrderByToken } from '@/lib/api/customer-orders';
import { getPublicQuoteByToken } from '@/lib/api/public-quotes';
import { requireCustomer } from '@/lib/auth/customer-flow';

export const generateMetadata = () => customerPageMetadata('title');

export default async function CustomerOrderPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ delivery?: string }>;
}) {
  const { id } = await params;
  const { delivery } = await searchParams;
  await requireCustomer(
    `${ROUTES.order(id)}${delivery ? `?delivery=${encodeURIComponent(delivery)}` : ''}`,
  );
  const order = await getCustomerOrder(id);
  const portal = await getCustomerAuthOptions();
  if (!order) notFound();
  const t = await getTranslations('customerOrders');
  const token = delivery || order.deliveryToken;
  if (delivery && (await getCustomerOrderByToken(delivery))?.id !== id) notFound();
  const quote = token ? await getPublicQuoteByToken(token) : null;
  return (
    <>
      <CustomerHeader
        section="orders"
        supplierName={portal.supplierName}
        logoUrl={portal.logoUrl}
      />
      {delivery && order.deliveryToken && delivery !== order.deliveryToken ? (
        <aside
          className="mx-auto w-full max-w-5xl px-4 pt-6 sm:px-6"
          aria-label={t('previousDelivery.title')}
        >
          <Card className="flex-row flex-wrap items-center justify-between px-5 py-4 gap-4">
            <div className="space-y-1">
              <p className="text-paragraph-sm-medium">{t('previousDelivery.title')}</p>
              <p className="text-paragraph-sm text-foreground-muted">
                {t('previousDelivery.description')}
              </p>
            </div>
            <Button asChild variant="outline">
              <PortalLink href={ROUTES.order(id)}>{t('previousDelivery.current')}</PortalLink>
            </Button>
          </Card>
        </aside>
      ) : null}
      {quote && token ? (
        <PublicQuoteView send={quote} token={token} />
      ) : (
        <main className="mx-auto flex w-full max-w-xl flex-col px-4 py-10 gap-y-6 sm:px-6">
          <h1 className="text-heading-4">{t('received')}</h1>
          <p className="text-paragraph text-foreground-muted">{t('preparing')}</p>
          <Card className="px-6 gap-y-3">
            <h2 className="text-heading-6">{t('original')}</h2>
            <p className="whitespace-pre-wrap break-words text-paragraph-sm">
              {order.originalText}
            </p>
          </Card>
        </main>
      )}
      <CustomerFooter />
    </>
  );
}
