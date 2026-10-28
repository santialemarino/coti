'use server';

import { getTranslations } from 'next-intl/server';

import { customerProfileSchema, type CustomerProfileValues } from '@/app/profile/form-schema';
import { ROUTES } from '@/config/routes';
import { customerFetch } from '@/lib/api/customer-orders';
import { requireCustomer } from '@/lib/auth/customer-flow';

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
): Promise<{ id?: string; error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  try {
    const response = await customerFetch('/profile/contacts/verification', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kind, value }),
    });
    if (!response.ok)
      return { error: t(response.status === 429 ? 'verificationWait' : 'verificationUnavailable') };
    const raw = (await response.json()) as { id: string };
    return { id: raw.id };
  } catch {
    return { error: t('verificationUnavailable') };
  }
}

export async function confirmContactVerification(
  id: string,
  token: string,
): Promise<{ error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  try {
    const response = await customerFetch('/profile/contacts/verification/confirm', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, token }),
    });
    if (response.ok) return {};
    return { error: t(response.status === 429 ? 'verificationLocked' : 'verificationInvalid') };
  } catch {
    return { error: t('verificationUnavailable') };
  }
}

export async function saveRecoveryEmail(email: string): Promise<{ error?: string }> {
  await requireCustomer(ROUTES.profile);
  const t = await getTranslations('customerProfile.errors');
  try {
    const response = await customerFetch('/auth/recovery-email', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.trim() }),
    });
    return response.ok
      ? {}
      : {
          error: t(
            response.status === 400 || response.status === 422
              ? 'invalid'
              : 'verificationUnavailable',
          ),
        };
  } catch {
    return { error: t('verificationUnavailable') };
  }
}
