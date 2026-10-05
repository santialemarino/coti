'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { BackLink } from '@/components/back-link';
import { ROUTES } from '@/config/routes';

// Below lg a settings page is opened from the sections list, so it names the way back to it.
export function SettingsBackLink() {
  const pathname = usePathname();
  const t = useTranslations('settings');

  if (pathname === ROUTES.settings) return null;
  return <BackLink href={ROUTES.settings} label={t('backToSections')} className="lg:hidden" />;
}
