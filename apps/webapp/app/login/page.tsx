import { redirect } from 'next/navigation';

import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';
import { getCustomerSession } from '@/lib/auth/customer-session';

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; method?: string; error?: string }>;
}) {
  const query = await searchParams;
  const next = safeCustomerDestination(query.next, ROUTES.orders);
  if (await getCustomerSession()) redirect(next);
  return (
    <CustomerAuthPage
      mode={query.method === 'password' ? 'login' : 'welcome'}
      next={next}
      initialError={
        ['googleFailed', 'googleUnavailable', 'unavailable'].includes(query.error || '')
          ? query.error
          : undefined
      }
    />
  );
}
