import { redirect } from 'next/navigation';

import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import {
  pendingGoogleEmail,
  pendingGoogleEmailProven,
  pendingGoogleSignup,
  safeCustomerDestination,
} from '@/lib/auth/customer-flow';
import { getCustomerSession } from '@/lib/auth/customer-session';

export default async function Page({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  if (!(await pendingGoogleSignup())) redirect(ROUTES.login);
  const session = await getCustomerSession();
  const query = await searchParams;
  return (
    <CustomerAuthPage
      mode={session ? 'linkGoogle' : 'google'}
      taxId={session?.taxId}
      googleEmail={session ? undefined : await pendingGoogleEmail()}
      googleEmailProven={await pendingGoogleEmailProven()}
      next={safeCustomerDestination(query.next, ROUTES.newRequest)}
    />
  );
}
