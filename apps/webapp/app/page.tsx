import { redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { getCustomerSession } from '@/lib/auth/customer-session';

export default async function HomePage() {
  redirect((await getCustomerSession()) ? ROUTES.orders : ROUTES.login);
}
