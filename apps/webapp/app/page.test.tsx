import { beforeEach, expect, it, vi } from 'vitest';

import HomePage from '@/app/page';
import { getCustomerSession } from '@/lib/auth/customer-session';

vi.mock('@/lib/auth/customer-session', () => ({ getCustomerSession: vi.fn() }));
vi.mock('next/navigation', () => ({
  redirect: (path: string) => {
    throw new Error('redirect:' + path);
  },
}));
beforeEach(() => vi.clearAllMocks());
it('takes returning customers to history', async () => {
  vi.mocked(getCustomerSession).mockResolvedValue({
    customerId: 'c',
    taxId: '20123456786',
    name: null,
    sessionExpiresAt: '2030-01-01T00:00:00Z',
  });
  await expect(HomePage()).rejects.toThrow('redirect:/orders');
});
it('takes visitors to login', async () => {
  vi.mocked(getCustomerSession).mockResolvedValue(null);
  await expect(HomePage()).rejects.toThrow('redirect:/login');
});
