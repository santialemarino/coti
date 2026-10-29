import { beforeEach, expect, it, vi } from 'vitest';

import { completeCustomerGoogle } from '@/app/login/actions';
import { pendingGoogleSignup, postCustomerAuth } from '@/lib/auth/customer-flow';
import messages from '@/translations/es.json';

vi.mock('next/headers', () => ({ cookies: vi.fn(), headers: vi.fn() }));
vi.mock('next/navigation', () => ({ redirect: vi.fn() }));
vi.mock('@/lib/auth/customer-flow', async (original) => ({
  ...(await original<typeof import('@/lib/auth/customer-flow')>()),
  pendingGoogleSignup: vi.fn(),
  postCustomerAuth: vi.fn(),
}));

function answer(status: number, body: object) {
  vi.mocked(postCustomerAuth).mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  );
}
const values = { taxId: '20123456786', name: 'Juan Perez', next: '/orders', useGoogleEmail: true };

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(pendingGoogleSignup).mockResolvedValue('p'.repeat(43));
});

it('tells a Google address verified on another account apart from a taken CUIT/CUIL', async () => {
  answer(409, { code: 'CONTACT_IN_USE', error: 'conflict' });
  const inUse = await completeCustomerGoogle(values);
  expect(inUse).toEqual({ error: 'contactInUse' });
  expect(messages.customerAuth.errors[inUse.error as 'contactInUse']).toBeTruthy();
  answer(409, { code: 'CONFLICT', error: 'conflict' });
  expect(await completeCustomerGoogle(values)).toEqual({ error: 'conflict' });
});
