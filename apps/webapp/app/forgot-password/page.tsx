import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import { safeCustomerDestination } from '@/lib/auth/customer-flow';

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; error?: string }>;
}) {
  const query = await searchParams;
  const next = safeCustomerDestination(query.next, ROUTES.orders);

  return (
    <CustomerAuthPage
      mode={'forgot'}
      next={next}
      initialError={
        ['googleFailed', 'googleUnavailable', 'unavailable'].includes(query.error || '')
          ? query.error
          : undefined
      }
    />
  );
}
