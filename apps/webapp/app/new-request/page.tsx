import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getTranslations } from 'next-intl/server';

import { InlineLink } from '@repo/ui/components';
import { PublicRfqSiteForm } from '@/app/_components/public-rfq-site-form';
import { ROUTES } from '@/config/routes';
import { getPublicRfqSite } from '@/lib/api/public-rfqs';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getCustomerHostname } from '@/lib/auth/customer-session';

export default async function NewRequestPage() {
  const session = await requireCustomer(ROUTES.newRequest);
  const site = await getPublicRfqSite(await getCustomerHostname());
  if (!site) notFound();
  const t = await getTranslations('customerOrders');
  return (
    <>
      <nav className="mx-auto w-full max-w-xl px-4 pt-6 sm:px-6">
        <InlineLink asChild>
          <Link href={ROUTES.orders}>{t('back')}</Link>
        </InlineLink>
      </nav>
      <PublicRfqSiteForm site={site} taxId={session.taxId} />
    </>
  );
}
