import { notFound } from 'next/navigation';

import { PublicRfqSiteForm } from '@/app/_components/public-rfq-site-form';
import { CustomerFooter } from '@/components/customer-footer';
import { CustomerHeader } from '@/components/customer-header';
import { ROUTES } from '@/config/routes';
import { customerPageMetadata, getCustomerAuthOptions } from '@/lib/api/customer-auth-options';
import { getCustomerProfile } from '@/lib/api/customer-profile';
import { getPublicRfqSite } from '@/lib/api/public-rfqs';
import { requireCustomer } from '@/lib/auth/customer-flow';
import { getCustomerHostname } from '@/lib/auth/customer-session';

export const generateMetadata = () => customerPageMetadata('newRequest');

export default async function NewRequestPage() {
  const session = await requireCustomer(ROUTES.newRequest);
  const portal = await getCustomerAuthOptions();
  const site = await getPublicRfqSite(await getCustomerHostname());
  const profile = await getCustomerProfile();
  if (!site) notFound();
  return (
    <>
      <CustomerHeader
        section="orders"
        supplierName={portal.supplierName}
        logoUrl={portal.logoUrl}
      />
      <PublicRfqSiteForm
        site={site}
        taxId={session.taxId}
        contact={{
          name: profile.name,
          email:
            profile.contacts.find((contact) => contact.kind === 'EMAIL' && contact.primary)
              ?.value ?? '',
          phone:
            profile.contacts.find((contact) => contact.kind === 'WHATSAPP' && contact.primary)
              ?.value ?? '',
        }}
      />
      <CustomerFooter />
    </>
  );
}
