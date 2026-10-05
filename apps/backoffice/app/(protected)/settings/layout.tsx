import { getTranslations } from 'next-intl/server';

import { SettingsNav } from '@/app/(protected)/settings/_components/settings-nav';
import { settingsNavItems } from '@/app/(protected)/settings/_components/settings-nav-items';
import { getBranches } from '@/lib/api/branches';
import { getOnboarding } from '@/lib/api/onboarding';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';

/*
 * The frame every settings page shares. From lg up the sections are listed in the context column;
 * below it they sit above the page, on the column's own white surface. The reads are the requests
 * the shell's layout already made, served again from the per-request cache.
 */
export default async function SettingsLayout({ children }: { children: React.ReactNode }) {
  const t = await getTranslations('settings');
  const session = await getSession();
  const isAdmin = session?.role === ADMIN_ROLE;
  const items = isAdmin
    ? settingsNavItems({
        t,
        tSetup: await getTranslations('common.setup'),
        onboarding: await getOnboarding(),
        branches: await getBranches(),
      })
    : [];

  return (
    <div className="flex min-w-0 flex-col px-6 py-10 gap-y-8 lg:px-10">
      {items.length > 0 ? (
        <div className="flex flex-col bg-background border border-border rounded-xl lg:hidden">
          <p className="pt-4 pb-2 pl-6 text-paragraph-xs-medium text-foreground-subtle uppercase">
            {t('title')}
          </p>
          <SettingsNav title={t('title')} items={items} />
        </div>
      ) : null}
      {children}
    </div>
  );
}
