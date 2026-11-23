import Link from 'next/link';
import { KeyRoundIcon, MailIcon } from 'lucide-react';
import { getTranslations } from 'next-intl/server';

import { Badge, Button, Separator } from '@repo/ui/components';
import { PageHeader } from '@/app/(protected)/_components/page-header';
import { AccountForm } from '@/app/(protected)/settings/account/_components/account-form';
import { ROUTES } from '@/config/routes';
import { getAccount } from '@/lib/api/account';
import { getARCASetup } from '@/lib/api/arca-setup';
import { getBranches } from '@/lib/api/branches';
import { requireAdmin } from '@/lib/auth/session';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('accountSettings');

export default async function AccountSettingsPage() {
  const session = await requireAdmin();
  const t = await getTranslations('account');
  const [account, setup, branches] = await Promise.all([
    getAccount(),
    getARCASetup(),
    getBranches(),
  ]);
  const arcaConnection = setup.hasCertificate ? 'connected' : setup.csr ? 'inProgress' : null;

  return (
    <main className="flex flex-col gap-y-8">
      <PageHeader title={t('title')} />
      <section
        aria-labelledby="incoming-mailboxes-title"
        className="flex w-full max-w-2xl flex-col gap-y-4"
      >
        <div className="flex flex-col gap-y-2">
          <h2 id="incoming-mailboxes-title" className="text-heading-6">
            {t('mailboxes.title')}
          </h2>
          <p className="text-paragraph-sm text-foreground-muted">{t('mailboxes.description')}</p>
        </div>
        <ul className="flex flex-col gap-y-3">
          {branches.map((branch) => (
            <li
              key={branch.id}
              className="flex flex-col items-start justify-between px-4 py-4 gap-y-3 bg-card border border-border rounded-lg sm:flex-row sm:items-center sm:gap-x-4"
            >
              <div className="flex min-w-0 flex-col gap-y-1">
                <h3 className="text-paragraph-sm-medium text-foreground">{branch.name}</h3>
                <p className="text-paragraph-sm text-foreground wrap-anywhere">
                  {branch.email ?? t('mailboxes.missing')}
                </p>
              </div>
              <Button asChild variant="outline" size="sm">
                <Link href={ROUTES.branchChannels(branch.id)}>{t('mailboxes.configure')}</Link>
              </Button>
            </li>
          ))}
        </ul>
      </section>
      <Separator />
      <section className="flex w-full max-w-xl flex-col gap-y-4">
        <div className="flex flex-col gap-y-2">
          <h2 className="text-heading-6">{t('access.title')}</h2>
          <p className="text-paragraph-sm text-foreground-muted">{t('access.description')}</p>
          <dl className="flex flex-col gap-y-1">
            <dt className="text-paragraph-sm-medium text-foreground">{t('access.email')}</dt>
            <dd className="flex flex-wrap items-center gap-x-3 gap-y-2">
              <span className="text-paragraph-sm text-foreground wrap-anywhere">
                {session.email}
              </span>
              <Badge tone={session.emailVerified ? 'success' : 'warning'} size="sm" dot>
                {t(session.emailVerified ? 'access.verified' : 'access.unverified')}
              </Badge>
            </dd>
          </dl>
        </div>
        <div className="flex flex-wrap items-center gap-x-6 gap-y-4">
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
      <Separator />
      <AccountForm account={account} arcaConnection={arcaConnection} />
    </main>
  );
}
