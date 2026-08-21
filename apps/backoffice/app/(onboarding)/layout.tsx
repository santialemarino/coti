import { redirect } from 'next/navigation';

import { ROUTES } from '@/config/routes';
import { requireAdmin } from '@/lib/auth/session';

export default async function OnboardingLayout({ children }: { children: React.ReactNode }) {
  const session = await requireAdmin();
  if (!session.emailVerified) redirect(ROUTES.verifyEmail);

  return <div className="min-h-screen bg-sunken">{children}</div>;
}
