import { beforeEach, expect, it, vi } from 'vitest';

import {
  confirmContactVerification,
  requestContactVerification,
  saveCustomerProfile,
} from '@/app/profile/actions';
import { customerFetch } from '@/lib/api/customer-orders';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: async () => (key: string) =>
    (messages.customerProfile.errors as Record<string, string>)[key] ?? key,
}));
vi.mock('@/lib/api/customer-orders', () => ({ customerFetch: vi.fn() }));
vi.mock('@/lib/auth/customer-flow', () => ({ requireCustomer: vi.fn() }));

const errors = messages.customerProfile.errors;
const profile = {
  name: 'Juan Perez',
  contacts: [{ kind: 'EMAIL' as const, value: 'juan@example.test', primary: true }],
};

function answer(status: number, body: object = {}) {
  vi.mocked(customerFetch).mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  );
}

beforeEach(() => vi.mocked(customerFetch).mockReset());

it('explains the contact cap instead of a generic invalid', async () => {
  answer(422, { code: 'CONTACT_LIMIT', error: 'invalid input' });
  expect(await saveCustomerProfile(profile)).toEqual({ error: errors.contactLimit });
});

it('asks to wait when profile saves are rate limited', async () => {
  answer(429, { code: 'RATE_LIMITED', retry_after_seconds: 30 });
  expect(await saveCustomerProfile(profile)).toEqual({ error: errors.tooManyRequests });
});

it('asks to wait when a code request is refused for the destination', async () => {
  answer(429, { code: 'ACCOUNT_LOCKED' });
  expect(await requestContactVerification('EMAIL', 'juan@example.test')).toEqual({
    error: errors.verificationWait,
  });
});

it('tells a locked destination apart from a wrong code', async () => {
  answer(429, { code: 'ACCOUNT_LOCKED' });
  expect(await confirmContactVerification('proof', '123456')).toEqual({
    error: errors.verificationLocked,
  });
  answer(401, { code: 'UNAUTHENTICATED' });
  expect(await confirmContactVerification('proof', '123456')).toEqual({
    error: errors.verificationInvalid,
  });
});
