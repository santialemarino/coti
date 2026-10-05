import Link from 'next/link';
import { KeyRoundIcon, MailIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Button, Separator } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { AccountForm } from '@/app/(protected)/settings/account/_components/account-form';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('accountSettings');

export default async function AccountSettingsPage() {
  await requireAdmin();
  const t = await getTranslations('account');
  const account = await getAccount();

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <AccountForm account={account} />
      <section className="flex flex-col max-w-md gap-y-4 lg:max-w-none">
        <Separator />
        {/* The actions follow the title on its own line, and drop below it only when they no longer fit. */}
        <div className="flex flex-wrap items-center gap-x-6 gap-y-4">
          <h2 className="text-heading-6">{t('access.title')}</h2>
          <div className="flex flex-wrap gap-x-3 gap-y-3">
            <Button asChild variant="outline">
              <Link href={ROUTES.emailSettings}>
                <MailIcon aria-hidden="true" />
                {t('access.changeEmail')}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link href={ROUTES.changePassword}>
                <KeyRoundIcon aria-hidden="true" />
                {t('access.changePassword')}
              </Link>
            </Button>
          </div>
        </div>
      </section>
    </main>
  );
}
