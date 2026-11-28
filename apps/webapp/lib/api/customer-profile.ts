import 'server-only';

import { customerFetch } from '@/lib/api/customer-orders';
import {
  mapCustomerProfile,
  type ContactKind,
  type CustomerProfile,
  type CustomerProfileRaw,
} from '@/lib/api/customer-profile-model';
import { CustomerAuthError } from '@/lib/auth/customer-session';

export async function getCustomerProfile(): Promise<CustomerProfile> {
  const response = await customerFetch('/profile');
  if (!response.ok) throw new CustomerAuthError(response.status);
  return mapCustomerProfile((await response.json()) as CustomerProfileRaw);
}

// --- Contact verification ---

interface ContactVerificationRaw {
  kind: ContactKind;
  value: string;
  status: ContactVerificationState;
  next: string | null;
  code?: string;
  whatsapp_link?: string;
}

export type ContactVerificationState = 'PENDING' | 'VERIFIED' | 'EXPIRED';

export interface ContactVerificationStatus {
  kind: ContactKind;
  value: string;
  status: ContactVerificationState;
  next: string | null;
  /* A pending WhatsApp proof: the message the customer sends, and the chat it goes to. */
  whatsappProof?: WhatsAppProof;
}

export interface WhatsAppProof {
  code: string;
  link: string;
}

const VERIFICATION_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/* One of the caller's own verifications; null when the id is malformed or not theirs. */
export async function getContactVerification(
  id: string,
): Promise<ContactVerificationStatus | null> {
  if (!VERIFICATION_ID.test(id)) return null;
  const response = await customerFetch(`/profile/contacts/verification/${id}`);
  if (response.status === 404 || response.status === 400) return null;
  if (!response.ok) throw new CustomerAuthError(response.status);
  const raw = (await response.json()) as ContactVerificationRaw;
  return {
    kind: raw.kind,
    value: raw.value,
    status: raw.status,
    next: raw.next,
    ...(raw.code && raw.whatsapp_link
      ? { whatsappProof: { code: raw.code, link: raw.whatsapp_link } }
      : {}),
  };
}
