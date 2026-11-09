import { ROUTES } from '@/config/routes';
import { getCustomerSession } from '@/lib/auth/customer-session';
import { customerRedirect } from '@/lib/portal/server';

export default async function HomePage() {
  return await customerRedirect((await getCustomerSession()) ? ROUTES.orders : ROUTES.login);
}
