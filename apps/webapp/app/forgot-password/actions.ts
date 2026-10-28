'use server';

import { redirect } from 'next/navigation';

import { postCustomerAuth, safeCustomerDestination } from '@/lib/auth/customer-flow';
import { acceptCustomerSession, type CustomerLoginRaw } from '@/lib/auth/customer-session';

export type RecoveryContactKind = 'EMAIL' | 'WHATSAPP';

interface RecoveryContactRaw {
  id: string;
  kind: RecoveryContactKind;
  masked: string;
  main: boolean;
  available: boolean;
  preselected: boolean;
}

export interface RecoveryContact {
  id: string;
  kind: RecoveryContactKind;
  masked: string;
  main: boolean;
  available: boolean;
  preselected: boolean;
}

export type RecoveryError = 'invalid' | 'wait' | 'locked' | 'code' | 'unavailable';

function errorForStatus(status: number): RecoveryError {
  if (status === 423) return 'wait';
  if (status === 429) return 'locked';
  if (status === 401) return 'code';
  if (status === 400 || status === 422) return 'invalid';
  return 'unavailable';
}

function mapRecoveryContact(raw: RecoveryContactRaw): RecoveryContact {
  return {
    id: raw.id,
    kind: raw.kind,
    masked: raw.masked,
    main: raw.main,
    available: raw.available,
    preselected: raw.preselected,
  };
}

export async function listRecoveryContacts(
  taxId: string,
): Promise<{ contacts?: RecoveryContact[]; error?: RecoveryError }> {
  try {
    const response = await postCustomerAuth('/auth/recovery/options', { tax_id: taxId });
    if (!response.ok) return { error: errorForStatus(response.status) };
    const raw = (await response.json()) as { contacts: RecoveryContactRaw[] };
    return { contacts: raw.contacts.map(mapRecoveryContact) };
  } catch {
    return { error: 'unavailable' };
  }
}

export async function sendRecoveryCode(
  taxId: string,
  contactId: string,
): Promise<{ id?: string; error?: RecoveryError }> {
  try {
    const response = await postCustomerAuth('/auth/recovery/code', {
      tax_id: taxId,
      contact_id: contactId,
    });
    if (!response.ok) return { error: errorForStatus(response.status) };
    return { id: ((await response.json()) as { id: string }).id };
  } catch {
    return { error: 'unavailable' };
  }
}

export async function resetWithRecoveryCode(values: {
  id: string;
  code: string;
  password: string;
  next: string;
}): Promise<{ error?: RecoveryError }> {
  try {
    const response = await postCustomerAuth('/auth/recovery/reset', {
      id: values.id,
      code: values.code,
      password: values.password,
    });
    if (!response.ok) return { error: errorForStatus(response.status) };
    await acceptCustomerSession((await response.json()) as CustomerLoginRaw);
  } catch {
    return { error: 'unavailable' };
  }
  redirect(safeCustomerDestination(values.next));
}
