'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { BackLink } from '@/components/back-link';
import { ROUTES } from '@/config/routes';

// The caller's own pages, opened from the profile menu rather than from the sections list.
const OWN_PAGES: readonly string[] = [ROUTES.changePassword, ROUTES.emailSettings];

// Below lg a settings page is opened from the sections list, so it names the way back to it.
export function SettingsBackLink() {
  const pathname = usePathname();
  const t = useTranslations('settings');

  if (pathname === ROUTES.settings || OWN_PAGES.includes(pathname)) return null;
  // The layout spaces its children 32px apart; the way back belongs to the title, 8px above it.
  return (
    <BackLink href={ROUTES.settings} label={t('backToSections')} className="-mb-6 lg:hidden" />
  );
}
