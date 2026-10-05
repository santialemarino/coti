import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { AppHeader } from '@/app/(protected)/_components/app-header';
import { ContextColumn } from '@/app/(protected)/_components/context-column';
import { PrimaryNav } from '@/app/(protected)/_components/primary-nav';
import { ScreenFade } from '@/app/(protected)/_components/screen-fade';
import { settingsNavItems } from '@/app/(protected)/_components/settings-nav-items';
import { RfqQueueProvider } from '@/app/(protected)/rfqs/_components/rfq-queue-provider';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getOnboarding } from '@/lib/api/onboarding';
import { getSelectedBranchId } from '@/lib/auth/branch';
import { getSession, mustVerifyEmail } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

// Middleware answers whether a token exists; this answers whether the session
// behind it is still good, which only the API knows.
export default async function ProtectedLayout({ children }: { children: React.ReactNode }) {
  const session = await getSession();
  if (!session) redirect(ROUTES.sessionEnded);
  // Ahead of every other read, which is itself a closed route and does not catch: asking one
  // first would answer 403 and throw, where the confirmation screen belonged.
  if (mustVerifyEmail(session)) redirect(ROUTES.verifyEmail);

  const isAdmin = session.role === ADMIN_ROLE;
  const [branches, onboarding] = await Promise.all([
    getBranches(),
    isAdmin ? getOnboarding() : null,
  ]);
  // A cookie naming a branch the caller no longer reaches makes every scoped read answer 403; a
  // layout cannot write cookies, so a route handler drops it.
  const selected = await getSelectedBranchId();
  if (selected && !branches.some((branch) => branch.id === selected)) {
    redirect(ROUTES.branchReset);
  }

  if (onboarding) {
    // Onboarding sends an account with no active branch to Sucursales; sending it back would loop.
    const canOnboard = branches.some((branch) => branch.isActive);
    if (onboarding.status === 'IN_PROGRESS' && canOnboard) redirect(ROUTES.onboarding);
  }

  const settingsNav = isAdmin
    ? settingsNavItems({
        t: await getTranslations('settings'),
        tSetup: await getTranslations('common.setup'),
        onboarding,
        branches,
      })
    : [];

  return (
    <div className="flex flex-col min-h-screen">
      <AppHeader session={session} />
      <RfqQueueProvider>
        {/* Below lg the tab bar covers the bottom of the screen, so the page ends above it. */}
        <div className="flex flex-1 items-stretch max-lg:pb-16">
          <ContextColumn settingsNav={settingsNav} />
          <ScreenFade>{children}</ScreenFade>
        </div>
      </RfqQueueProvider>
      <div
        data-slot="tab-bar"
        className="fixed inset-x-0 bottom-0 z-40 bg-background/85 border-t border-border backdrop-blur [view-transition-name:shell-tabs] lg:hidden"
      >
        <PrimaryNav layout="tabs" />
      </div>
    </div>
  );
}
