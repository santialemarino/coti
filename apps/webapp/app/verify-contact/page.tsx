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
    /* The proofs sign-up sent, by id, so the screen only says what is really on its way. */
    email?: string;
    phone?: string;
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
    const verification = await getContactVerification(query.challenge || '');
    const state: EmailConfirmationState =
      !verification || verification.kind !== 'EMAIL' || verification.status === 'EXPIRED'
        ? 'invalid'
        : verification.status === 'VERIFIED'
          ? 'verified'
          : 'pending';
    const next = verification?.next ? safeCustomerDestination(verification.next, '') : '';
    return (
      <CustomerAuthShell supplierName={portal.supplierName} logoUrl={portal.logoUrl}>
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
  const [profile, sentEmail, sentPhone] = await Promise.all([
    getCustomerProfile(),
    query.email ? getContactVerification(query.email) : Promise.resolve(null),
    query.phone ? getContactVerification(query.phone) : Promise.resolve(null),
  ]);
  if (!profile.ownershipUnproved && profile.contacts.some((contact) => contact.verified))
    return await customerRedirect(next);
  return (
    <CustomerAuthShell supplierName={portal.supplierName} logoUrl={portal.logoUrl}>
      <PendingVerification
        contacts={profile.contacts.filter((contact) => !contact.verified)}
        next={next}
        supplierName={portal.supplierName}
        ownershipUnproved={profile.ownershipUnproved}
        sentEmail={
          sentEmail?.kind === 'EMAIL' && sentEmail.status === 'PENDING'
            ? sentEmail.value
            : undefined
        }
        sentProof={
          sentPhone?.kind === 'WHATSAPP' &&
          sentPhone.status === 'PENDING' &&
          sentPhone.whatsappProof
            ? { id: query.phone || '', ...sentPhone.whatsappProof }
            : undefined
        }
      />
    </CustomerAuthShell>
  );
}
