import { redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';

export default async function LegacyRequestPage() {
  redirect(ROUTES.newRequest);
}
