import { notFound } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { getCustomerOrderByToken, getDeliveryHostname } from '@/lib/api/customer-orders';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getCustomerHostname } from '@/lib/auth/customer-session';
import { customerPortalURL, customerRedirect } from '@/lib/portal/server';

export default async function PublicQuotePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const hostname = await getDeliveryHostname(token);
  if (!hostname) notFound();
  if (hostname !== (await getCustomerHostname()))
    return await customerRedirect(await customerPortalURL(hostname, ROUTES.quote(token)));
  await requireCustomer(ROUTES.quote(token));
  const order = await getCustomerOrderByToken(token);
  if (!order) notFound();
  return await customerRedirect(ROUTES.order(order.id) + '?delivery=' + encodeURIComponent(token));
}
