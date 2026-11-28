import { render, screen } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import CustomerOrderPage from '@/app/orders/[id]/page';
import { getCustomerOrder, getCustomerOrderByToken } from '@/lib/api/customer-orders';
import { getPublicQuoteByToken } from '@/lib/api/public-quotes';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({
  notFound: () => {
    throw new Error('not found');
  },
}));
vi.mock('next-intl/server', () => ({
  getTranslations: async () =>
    createTranslator({ locale: 'es-AR', messages, namespace: 'customerOrders' }),
}));
vi.mock('@/lib/api/customer-orders', () => ({
  getCustomerOrder: vi.fn(),
  getCustomerOrderByToken: vi.fn(),
}));
vi.mock('@/lib/api/public-quotes', () => ({ getPublicQuoteByToken: vi.fn() }));
vi.mock('@/lib/auth/customer-flow', () => ({ requireCustomer: vi.fn() }));
vi.mock('@/components/customer-header', () => ({ CustomerHeader: () => null }));
vi.mock('@/components/customer-footer', () => ({ CustomerFooter: () => null }));
vi.mock('@/app/quotes/_components/public-quote-view', () => ({
  PublicQuoteView: ({ token }: { token: string }) => <div data-testid="publication">{token}</div>,
}));
vi.mock('@/lib/api/customer-auth-options', () => ({
  customerPageMetadata: vi.fn(),
  getCustomerAuthOptions: async () => ({ supplierName: 'Supplier' }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getCustomerOrder).mockResolvedValue({
    id: 'order',
    deliveryToken: 'current',
  } as Awaited<ReturnType<typeof getCustomerOrder>>);
  vi.mocked(getCustomerOrderByToken).mockResolvedValue({ id: 'order' } as Awaited<
    ReturnType<typeof getCustomerOrderByToken>
  >);
  vi.mocked(getPublicQuoteByToken).mockResolvedValue({ status: 'ACTIVE' } as Awaited<
    ReturnType<typeof getPublicQuoteByToken>
  >);
});

it('preserves an authorized historical publication and offers the current order', async () => {
  render(
    await CustomerOrderPage({
      params: Promise.resolve({ id: 'order' }),
      searchParams: Promise.resolve({ delivery: 'historical' }),
    }),
  );
  expect(screen.getByTestId('publication').textContent).toBe('historical');
  expect(
    screen
      .getByRole('link', { name: messages.customerOrders.previousDelivery.current })
      .getAttribute('href'),
  ).toBe('/orders/order');
  expect(getPublicQuoteByToken).toHaveBeenCalledWith('historical');
});

it('loads the latest delivery on the canonical order without a historical warning', async () => {
  render(
    await CustomerOrderPage({
      params: Promise.resolve({ id: 'order' }),
      searchParams: Promise.resolve({}),
    }),
  );
  expect(screen.getByTestId('publication').textContent).toBe('current');
  expect(
    screen.queryByRole('link', { name: messages.customerOrders.previousDelivery.current }),
  ).toBeNull();
});

it('does not resolve a publication belonging to another order', async () => {
  vi.mocked(getCustomerOrderByToken).mockResolvedValue({ id: 'another' } as Awaited<
    ReturnType<typeof getCustomerOrderByToken>
  >);
  await expect(
    CustomerOrderPage({
      params: Promise.resolve({ id: 'order' }),
      searchParams: Promise.resolve({ delivery: 'foreign' }),
    }),
  ).rejects.toThrow('not found');
  expect(getPublicQuoteByToken).not.toHaveBeenCalled();
});
