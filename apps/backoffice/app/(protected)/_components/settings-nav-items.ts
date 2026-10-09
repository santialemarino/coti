import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { ROUTES } from '@/config/routes';
import type { Branch } from '@/lib/api/branches';
import type { Onboarding } from '@/lib/api/onboarding';
import type { MessageFor } from '@/lib/forms/validators';
import { hasPendingChecklist } from '@/lib/utils/onboarding-checklist';
import { anyBranchMissesEmail } from '@/lib/utils/setup-issues';

interface SettingsNavInput {
  // The `settings` namespace and the `common.setup` one.
  t: MessageFor;
  tSetup: MessageFor;
  onboarding: Onboarding | null;
  branches: Branch[];
}

/*
 * The settings sections an admin is offered. Every admin page still refuses a seller on its own, so
 * leaving a seller without entries is a courtesy, not the guard.
 */
export function settingsNavItems({
  t,
  tSetup,
  onboarding,
  branches,
}: SettingsNavInput): SettingsNavItem[] {
  return [
    { href: ROUTES.accountSettings, label: t('nav.account') },
    {
      href: ROUTES.branchSettings,
      label: t('nav.branches'),
      attention: anyBranchMissesEmail(branches) ? tSetup('attention') : undefined,
    },
    { href: ROUTES.userSettings, label: t('nav.users') },
    { href: ROUTES.catalogSettings, label: t('nav.catalog') },
    { href: ROUTES.priceSettings, label: t('nav.prices') },
    { href: ROUTES.invoicingSettings, label: t('nav.invoicing') },
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
  ];
}
