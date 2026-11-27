import { getTranslations } from 'next-intl/server';

import {
  ContactEmailConfirmation,
  type EmailConfirmationState,
} from '@/app/verify-contact/_components/contact-email-confirmation';
import { PendingVerification } from '@/app/verify-contact/_components/pending-verification';
import { CustomerAuthShell } from '@/components/customer-auth-shell';
import { ROUTES } from '@/config/routes';
import { customerPageMetadata, getCustomerAuthOptions } from '@/lib/api/customer-auth-options';
import { getContactVerification, getCustomerProfile } from '@/lib/api/customer-profile';
import { requireCustomer, safeCustomerDestination } from '@/lib/auth/customer-flow';
import { customerRedirect } from '@/lib/portal/server';

export const generateMetadata = () => customerPageMetadata('verifyContact');

export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{
    challenge?: string;
    token?: string;
    next?: string;
    verification?: string;
  }>;
}) {
  const query = await searchParams;
  const linked = Boolean(query.challenge && query.token);
  const destination =
    ROUTES.verifyContact +
    '?' +
    new URLSearchParams(
      linked
        ? { challenge: query.challenge || '', token: query.token || '' }
        : query.next
          ? { next: query.next }
          : {},
    );
  await requireCustomer(destination);
  const portal = await getCustomerAuthOptions();

  // An emailed link: confirm the proof it carries, and return where the request asked.
  if (linked) {
    const [verification, t] = await Promise.all([
      getContactVerification(query.challenge || ''),
      getTranslations('customerVerification'),
    ]);
    const state: EmailConfirmationState =
      !verification || verification.kind !== 'EMAIL' || verification.status === 'EXPIRED'
        ? 'invalid'
        : verification.status === 'VERIFIED'
          ? 'verified'
          : 'pending';
    const next = verification?.next ? safeCustomerDestination(verification.next, '') : '';
    return (
      <CustomerAuthShell supplierName={portal.supplierName} logoUrl={portal.logoUrl}>
        <h1 className="sr-only">{t('confirm.title')}</h1>
        <ContactEmailConfirmation
          challenge={query.challenge || ''}
          token={query.token || ''}
          email={verification?.value}
          next={next || undefined}
          supplierName={portal.supplierName}
          initialState={state}
        />
      </CustomerAuthShell>
    );
  }

  // After sign-up, or from a quote the account cannot open yet: prove a contact, then continue.
  const next = safeCustomerDestination(query.next, ROUTES.orders);
  const [profile, sent] = await Promise.all([
    getCustomerProfile(),
    query.verification ? getContactVerification(query.verification) : Promise.resolve(null),
  ]);
  if (!profile.ownershipUnproved && profile.contacts.some((contact) => contact.verified))
    return await customerRedirect(next);
  return (
    <CustomerAuthShell supplierName={portal.supplierName} logoUrl={portal.logoUrl}>
      <PendingVerification
        contacts={profile.contacts.filter((contact) => !contact.verified)}
        next={next}
        supplierName={portal.supplierName}
        sentCode={
          sent?.kind === 'WHATSAPP' && sent.status === 'PENDING'
            ? { value: sent.value, challenge: query.verification || '' }
            : undefined
        }
      />
    </CustomerAuthShell>
  );
}
