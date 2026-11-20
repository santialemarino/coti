import { notFound } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { getCustomerOrderByToken, getDeliveryHostname } from '@/lib/api/customer-orders';
import { getCustomerProfile } from '@/lib/api/customer-profile';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getCustomerHostname } from '@/lib/auth/customer-session';
import { customerPortalURL, customerRedirect } from '@/lib/portal/server';

export default async function PublicQuotePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const hostname = await getDeliveryHostname(token);
  if (!hostname) notFound();
  if (hostname !== (await getCustomerHostname())) {
    // Redirecting to a hostname outside the portal base would land back here and loop.
    const target = customerPortalURL(hostname, ROUTES.quote(token));
    if (!target) notFound();
    return await customerRedirect(target);
  }
  await requireCustomer(ROUTES.quote(token));
  const order = await getCustomerOrderByToken(token);
  if (!order) {
    const profile = await getCustomerProfile();
    if (profile.ownershipUnproved || !profile.contacts.some((contact) => contact.verified))
      return await customerRedirect(ROUTES.orders);
    notFound();
  }
  return await customerRedirect(ROUTES.order(order.id) + '?delivery=' + encodeURIComponent(token));
}
