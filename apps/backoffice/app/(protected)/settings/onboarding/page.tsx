import { getTranslations } from 'next-intl/server';

import { OnboardingChecklist } from '@/app/(protected)/_components/onboarding-checklist';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { OperationalReadinessCard } from '@/app/(protected)/settings/onboarding/_components/operational-readiness';
import { getOnboarding, getOperationalReadiness } from '@/lib/api/onboarding';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('onboardingSettings');

export default async function OnboardingSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('onboarding.checklist');
  const [onboarding, readiness] = await Promise.all([getOnboarding(), getOperationalReadiness()]);

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('settingsTitle')} />
      <OperationalReadinessCard readiness={readiness} />
      <OnboardingChecklist onboarding={onboarding} placement="settings" />
    </main>
  );
}
