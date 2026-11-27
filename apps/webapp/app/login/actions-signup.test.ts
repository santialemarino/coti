import { beforeEach, expect, it, vi } from 'vitest';

import { signUpCustomer } from '@/app/login/actions';
import { customerFetch } from '@/lib/api/customer-orders';
import { startCustomerSession } from '@/lib/auth/customer-session';

vi.mock('next/headers', () => ({ headers: async () => new Headers() }));
vi.mock('@/lib/api/customer-orders', () => ({ customerFetch: vi.fn() }));
vi.mock('@/lib/auth/customer-session', () => ({
  CustomerAuthError: class extends Error {
    constructor(public status: number) {
      super(String(status));
    }
  },
  startCustomerSession: vi.fn(),
  acceptCustomerSession: vi.fn(),
  endCustomerSession: vi.fn(),
}));
vi.mock('@/lib/portal/server', () => ({
  customerRedirect: (path: string) => {
    throw new Error('redirect:' + path);
  },
}));

const values = {
  name: 'Obra Nueva',
  taxId: '20123456786',
  password: 'Customer-pass123!',
  email: 'obra@cliente.test',
  phone: '+5491155550000',
};

function proof(id: string | null) {
  return id
    ? new Response(JSON.stringify({ id }), { status: 200 })
    : new Response('{}', { status: 503 });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(startCustomerSession).mockResolvedValue({} as never);
});

it('sends both proofs at once with the quote to return to, then asks for one', async () => {
  vi.mocked(customerFetch).mockImplementation(async (_path, init) =>
    proof(JSON.parse(String(init?.body)).kind === 'EMAIL' ? 'e-1' : 'p-1'),
  );
  await expect(signUpCustomer({ ...values, next: '/quotes/tok-1' })).rejects.toThrow(
    'redirect:/verify-contact?next=%2Fquotes%2Ftok-1&email=e-1&phone=p-1',
  );
  const bodies = vi
    .mocked(customerFetch)
    .mock.calls.map(([, init]) => JSON.parse(String(init?.body)));
  expect(bodies).toEqual([
    { kind: 'EMAIL', value: 'obra@cliente.test', next: '/quotes/tok-1' },
    { kind: 'WHATSAPP', value: '+5491155550000', next: '/quotes/tok-1' },
  ]);
});

it('names only the proofs that went out', async () => {
  vi.mocked(customerFetch).mockImplementation(async (_path, init) =>
    proof(JSON.parse(String(init?.body)).kind === 'EMAIL' ? 'e-1' : null),
  );
  await expect(signUpCustomer({ ...values, next: '/quotes/tok-1' })).rejects.toThrow(
    'redirect:/verify-contact?next=%2Fquotes%2Ftok-1&email=e-1',
  );
});

it('still sends the proofs but goes straight on when nothing earlier is waiting', async () => {
  vi.mocked(customerFetch).mockResolvedValue(proof('e-1'));
  await expect(signUpCustomer({ ...values, phone: '', next: '/new-request' })).rejects.toThrow(
    'redirect:/new-request',
  );
  expect(customerFetch).toHaveBeenCalledTimes(1);
});

it('reports a refused registration without sending anything', async () => {
  const { CustomerAuthError } = await import('@/lib/auth/customer-session');
  vi.mocked(startCustomerSession).mockRejectedValue(new CustomerAuthError(409));
  await expect(signUpCustomer({ ...values, next: '/quotes/tok-1' })).resolves.toEqual({
    error: 'conflict',
  });
  expect(customerFetch).not.toHaveBeenCalled();
});
