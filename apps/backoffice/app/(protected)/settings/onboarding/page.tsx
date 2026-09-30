import { redirect } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { ROUTES } from '@/config/routes';
import { getOnboarding } from '@/lib/api/onboarding';
import { requireAdmin } from '@/lib/auth/session';
import { hasPendingChecklist } from '@/lib/utils/onboarding-checklist';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('onboardingSettings');

export default async function OnboardingSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('onboarding.checklist');
  const onboarding = await getOnboarding();
  // The rail offers this page only while a step is pending; a stale link lands on the account.
  if (!hasPendingChecklist(onboarding)) redirect(ROUTES.accountSettings);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('settingsTitle')} />
      <OnboardingChecklist onboarding={onboarding} placement="settings" className="max-w-2xl" />
    </main>
  );
}
