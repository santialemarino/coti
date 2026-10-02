import { getTranslations } from 'next-intl/server';

import {
  SettingsNav,
  type SettingsNavItem,
} from '@/app/(protected)/settings/_components/settings-nav';
import { ROUTES } from '@/config/routes';
import { getBranches } from '@/lib/api/branches';
import { getOnboarding } from '@/lib/api/onboarding';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { hasPendingChecklist } from '@/lib/utils/onboarding-checklist';
import { anyBranchMissesEmail } from '@/lib/utils/setup-issues';

/*
 * The frame every settings page shares. The gate above already guarantees a session; this
 * reads it only to decide which entries the caller is offered — each admin page still refuses
 * a seller on its own, so a hidden entry is a courtesy, not the guard.
 */
export default async function SettingsLayout({ children }: { children: React.ReactNode }) {
  const t = await getTranslations('settings');
  const tSetup = await getTranslations('common.setup');
  const session = await getSession();
  const isAdmin = session?.role === ADMIN_ROLE;
  const [onboarding, branches] = isAdmin
    ? await Promise.all([getOnboarding(), getBranches()])
    : [null, []];

  const items: SettingsNavItem[] = [
    ...(isAdmin
      ? [
          { href: ROUTES.accountSettings, label: t('nav.account') },
          {
            href: ROUTES.branchSettings,
            label: t('nav.branches'),
            attention: anyBranchMissesEmail(branches) ? tSetup('attention') : undefined,
          },
          { href: ROUTES.userSettings, label: t('nav.users') },
          { href: ROUTES.catalogSettings, label: t('nav.catalog') },
          { href: ROUTES.priceSettings, label: t('nav.prices') },
          ...(onboarding && hasPendingChecklist(onboarding)
            ? [
                {
                  href: ROUTES.onboardingSettings,
                  label: t('nav.onboarding', {
                    done: onboarding.checklist.filter((item) => item.done).length,
                    total: onboarding.checklist.length,
                  }),
                },
              ]
            : []),
        ]
      : []),
  ];

  return (
    <div className="flex flex-col px-6 py-10 gap-y-8 lg:flex-row lg:px-10 lg:gap-x-12">
      <SettingsNav title={t('title')} items={items} />
      <div className="min-w-0 flex-1">{children}</div>
    </div>
  );
}
