'use server';

import { getTranslations } from 'next-intl/server';

import { customerProfileSchema, type CustomerProfileValues } from '@/app/profile/form-schema';
import { ROUTES } from '@/config/routes';
import { customerFetch } from '@/lib/api/customer-orders';
import {
  getContactVerification,
  type ContactVerificationState,
  type WhatsAppProof,
} from '@/lib/api/customer-profile';
import { requireCustomer, safeCustomerDestination } from '@/lib/auth/customer-flow';

export async function saveCustomerProfile(
  values: CustomerProfileValues,
): Promise<{ error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  const parsed = customerProfileSchema(t).safeParse(values);
  if (!parsed.success) return { error: t('invalid') };
  try {
    const response = await customerFetch('/profile', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: parsed.data.name,
        legal_name: parsed.data.legalName || null,
        fiscal_address: parsed.data.address || null,
        iva_condition: parsed.data.ivaCondition || null,
        contacts: parsed.data.contacts.filter((contact) => contact.value !== ''),
      }),
    });
    if (!response.ok) {
      const code =
        response.status === 422 ? ((await response.json()) as { code?: string }).code : undefined;
      return {
        error: t(
          code === 'CONTACT_LIMIT'
            ? 'contactLimit'
            : response.status === 429
              ? 'tooManyRequests'
              : response.status === 409
                ? 'conflict'
                : response.status === 400 || response.status === 422
                  ? 'invalid'
                  : 'unavailable',
        ),
      };
    }
    return {};
  } catch {
    return { error: t('unavailable') };
  }
}

export async function requestContactVerification(
  kind: 'EMAIL' | 'WHATSAPP',
  value: string,
  next?: string,
): Promise<{ id?: string; whatsappProof?: WhatsAppProof; error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  try {
    const response = await customerFetch('/profile/contacts/verification', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      // The API keeps the page to return to with the proof; the emailed link never carries it.
      body: JSON.stringify({ kind, value, next: next ? safeCustomerDestination(next, '') : '' }),
    });
    if (response.status === 429) return { error: t('verificationWait') };
    if (!response.ok) {
      const code = (await response.json().catch(() => ({}))) as { code?: string };
      return {
        error: t(
          code.code === 'PORTAL_NOT_CONFIGURED'
            ? 'verificationNotConfigured'
            : 'verificationUnavailable',
        ),
      };
    }
    const raw = (await response.json()) as { id: string; code?: string; whatsapp_link?: string };
    return {
      id: raw.id,
      ...(raw.code && raw.whatsapp_link
        ? { whatsappProof: { code: raw.code, link: raw.whatsapp_link } }
        : {}),
    };
  } catch {
    return { error: t('verificationUnavailable') };
  }
}

/* Where a WhatsApp proof stands, for the screen that waits on the customer's message. */
export async function readContactVerification(
  id: string,
): Promise<ContactVerificationState | null> {
  await requireCustomer(ROUTES.profile);
  try {
    return (await getContactVerification(id))?.status ?? null;
  } catch {
    return null;
  }
}

export async function confirmContactVerification(
  id: string,
  token: string,
): Promise<{ error?: string; next?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  try {
    const response = await customerFetch('/profile/contacts/verification/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, token }),
    });
    if (response.ok) {
      const raw = (await response.json().catch(() => ({}))) as { next?: string | null };
      const next = raw.next ? safeCustomerDestination(raw.next, '') : '';
      return next ? { next } : {};
    }
    return { error: t(response.status === 429 ? 'verificationLocked' : 'verificationInvalid') };
  } catch {
    return { error: t('verificationUnavailable') };
  }
}
