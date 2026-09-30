import { redirect } from 'next/navigation';

import { AppHeader } from '@/app/(protected)/_components/app-header';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getOnboarding } from '@/lib/api/onboarding';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

// Middleware answers whether a token exists; this answers whether the session
// behind it is still good, which only the API knows.
export default async function ProtectedLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  if (!session) redirect(ROUTES.sessionEnded);
  // Ahead of the onboarding read, which is itself a closed route and does not catch: asking it
  // first would answer 403 and throw, where the confirmation screen belonged.
  if (!session.emailVerified) redirect(ROUTES.verifyEmail);
  if (session.role === ADMIN_ROLE) {
    const [onboarding, branches] = await Promise.all([getOnboarding(), getBranches()]);
    // Onboarding sends an account with no active branch to Sucursales; sending it back would loop.
    const canOnboard = branches.some((branch) => branch.isActive);
    if (onboarding.status === 'IN_PROGRESS' && canOnboard) redirect(ROUTES.onboarding);
  }

  return (
    <div className="flex flex-col min-h-screen">
      <AppHeader session={session} />
      {children}
    </div>
  );
}
