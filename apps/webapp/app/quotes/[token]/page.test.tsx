import { beforeEach, expect, it, vi } from 'vitest';

import PublicQuotePage from '@/app/quotes/[token]/page';
import { getCustomerOrderByToken, getDeliveryHostname } from '@/lib/api/customer-orders';
import { requireCustomer } from '@/lib/auth/customer-flow';

vi.mock('@/lib/api/customer-orders', () => ({
  getCustomerOrderByToken: vi.fn(),
  getDeliveryHostname: vi.fn(),
}));
vi.mock('@/lib/auth/customer-flow', () => ({ requireCustomer: vi.fn() }));
vi.mock('next/navigation', () => ({
  redirect: (path: string) => {
    throw new Error('redirect:' + path);
  },
  notFound: () => {
    throw new Error('not-found');
  },
}));
vi.mock('@/lib/auth/customer-session', () => ({
  getCustomerHostname: async () => 'supplier.coti.test',
}));
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getDeliveryHostname).mockResolvedValue('supplier.coti.test');
});
it('authenticates before resolving an old link', async () => {
  vi.mocked(requireCustomer).mockRejectedValue(new Error('login-required'));
  await expect(
    PublicQuotePage({ params: Promise.resolve({ token: 'legacy-token' }) }),
  ).rejects.toThrow('login-required');
  expect(requireCustomer).toHaveBeenCalledWith('/quotes/legacy-token');
  expect(getCustomerOrderByToken).not.toHaveBeenCalled();
});
it('preserves the original link expiry reference in the unified detail', async () => {
  vi.mocked(requireCustomer).mockResolvedValue({
    customerId: 'c',
    taxId: '20123456786',
    name: null,
    sessionExpiresAt: '2030-01-01T00:00:00Z',
  });
  vi.mocked(getCustomerOrderByToken).mockResolvedValue({
    id: 'order-id',
    branchName: 'Branch',
    receivedAt: '',
    status: 'SENT',
    originalText: '',
    deliveryToken: 'newer-token',
  });
  await expect(
    PublicQuotePage({ params: Promise.resolve({ token: 'legacy-token' }) }),
  ).rejects.toThrow('redirect:/orders/order-id?delivery=legacy-token');
});
it('does not reveal an unauthorized request', async () => {
  vi.mocked(requireCustomer).mockResolvedValue({
    customerId: 'c',
    taxId: '20123456786',
    name: null,
    sessionExpiresAt: '2030-01-01T00:00:00Z',
  });
  vi.mocked(getCustomerOrderByToken).mockResolvedValue(null);
  await expect(
    PublicQuotePage({ params: Promise.resolve({ token: 'other-token' }) }),
  ).rejects.toThrow('not-found');
});
