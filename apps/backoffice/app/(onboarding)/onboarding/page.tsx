import { redirect } from 'next/navigation';

import { OnboardingFlow } from '@/app/(onboarding)/onboarding/_components/onboarding-flow';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import { getBranches } from '@/lib/api/branches';
import { getOnboarding } from '@/lib/api/onboarding';
import { getUsers } from '@/lib/api/users';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('onboarding');

export default async function OnboardingPage() {
  const session = await requireAdmin();
  const [onboarding, account, branches, users] = await Promise.all([
    getOnboarding(),
    getAccount(),
    getBranches(),
    getUsers(),
  ]);

  const firstBranch = branches.find((branch) => branch.isActive);
  if (!firstBranch) redirect(ROUTES.branchSettings);

  return (
    <OnboardingFlow
      onboarding={onboarding}
      account={account}
      branch={firstBranch}
      branches={branches.filter((branch) => branch.isActive)}
      users={users}
      currentUserId={session.userId}
    />
  );
}
