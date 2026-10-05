import { redirect } from 'next/navigation';
import { SettingsIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { EmptyState } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { getSession } from '@/lib/auth/session';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('settings');

/*
 * The settings index, the counterpart of the queue's landing: from lg up the column lists the
 * sections beside this empty pane; below it the column is the page and the screen steps aside.
 */
export default async function SettingsIndexPage() {
  const session = await getSession();
  // A seller has one settings page and no sections to choose from.
  if (session?.role !== ADMIN_ROLE) redirect(ROUTES.changePassword);
  const t = await getTranslations('settings');

  return (
    <div className="flex flex-1 items-center justify-center py-16">
      <EmptyState icon={SettingsIcon} title={t('title')} description={t('indexHint')} />
    </div>
  );
}
