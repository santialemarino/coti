import { ROUTES } from '@/config/routes';
import { customerRedirect } from '@/lib/portal/server';

export default async function LegacyRequestPage() {
  return await customerRedirect(ROUTES.newRequest);
}
