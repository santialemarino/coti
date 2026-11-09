import { CustomerAuthPage } from '@/components/customer-auth-page';
import { ROUTES } from '@/config/routes';
import {
  pendingGoogleEmail,
  pendingGoogleSignup,
  safeCustomerDestination,
} from '@/lib/auth/customer-flow';
import { getCustomerSession } from '@/lib/auth/customer-session';
import { customerRedirect } from '@/lib/portal/server';

export default async function Page({ searchParams }: { searchParams: Promise<{ next?: string }> }) {
  if (!(await pendingGoogleSignup())) return await customerRedirect(ROUTES.login);
  const session = await getCustomerSession();
  const query = await searchParams;
  return (
    <CustomerAuthPage
      mode={session ? 'linkGoogle' : 'google'}
      taxId={session?.taxId}
      googleEmail={session ? undefined : await pendingGoogleEmail()}
      next={safeCustomerDestination(query.next, ROUTES.newRequest)}
    />
  );
}
