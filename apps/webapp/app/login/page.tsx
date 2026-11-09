import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';
import { getCustomerSession } from '@/lib/auth/customer-session';
import { customerRedirect } from '@/lib/portal/server';

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; method?: string; error?: string }>;
}) {
  const query = await searchParams;
  const next = safeCustomerDestination(query.next, ROUTES.orders);
  if (await getCustomerSession()) return await customerRedirect(next);
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
