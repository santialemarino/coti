import { redirect } from 'next/navigation';

import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';
import { getCustomerSession } from '@/lib/auth/customer-session';

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; method?: string; error?: string; token?: string }>;
}) {
  const query = await searchParams;
  const next = safeCustomerDestination(query.next, ROUTES.newRequest);
  if (await getCustomerSession()) redirect(next);
  return (
    <CustomerAuthPage
      mode={'signup'}
      next={next}
      token={query.token}
      initialError={
        ['googleFailed', 'googleUnavailable', 'unavailable'].includes(query.error || '')
          ? query.error
          : undefined
      }
    />
  );
}
