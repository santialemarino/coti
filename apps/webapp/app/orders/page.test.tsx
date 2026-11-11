import { render, screen } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { beforeEach, expect, it, vi } from 'vitest';

import OrdersPage from '@/app/orders/page';
import { getCustomerOrders } from '@/lib/api/customer-orders';
import { getCustomerProfile } from '@/lib/api/customer-profile';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: async () =>
    createTranslator({ locale: 'es-AR', messages, namespace: 'customerOrders' }),
}));
vi.mock('@/lib/api/customer-orders', () => ({ getCustomerOrders: vi.fn() }));
vi.mock('@/lib/api/customer-profile', () => ({ getCustomerProfile: vi.fn() }));
vi.mock('@/lib/auth/customer-flow', () => ({ requireCustomer: vi.fn() }));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: async () => ({ timestamp: () => 'Date' }),
}));
vi.mock('@/app/login/actions', () => ({ signOutCustomer: vi.fn() }));
vi.mock('@/components/customer-header', () => ({ CustomerHeader: () => null }));
vi.mock('@/components/customer-footer', () => ({ CustomerFooter: () => null }));
vi.mock('@/lib/api/customer-auth-options', () => ({
  getCustomerAuthOptions: async () => ({
    supplierName: 'Corralón',
    passwordMinLength: 12,
    googleClientId: '',
  }),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getCustomerOrders).mockResolvedValue([]);
  vi.mocked(getCustomerProfile).mockResolvedValue({
    clientId: 'profile',
    taxId: '20123456786',
    name: 'Customer',
    ownershipUnproved: true,
    contacts: [{ kind: 'EMAIL', value: 'customer@example.test', primary: true, verified: false }],
  });
});

it('explains how to unlock an earlier profile instead of claiming it has no orders', async () => {
  render(await OrdersPage({ searchParams: Promise.resolve({}) }));
  expect(screen.getByText(messages.customerOrders.proveEarlierContact)).toBeDefined();
  expect(screen.queryByText(messages.customerOrders.empty)).toBeNull();
  expect(
    screen.getByRole('link', { name: messages.customerOrders.verifyContacts }).getAttribute('href'),
  ).toBe('/profile');
});

it('asks a customer with no verified contact to verify the one on the order', async () => {
  vi.mocked(getCustomerProfile).mockResolvedValue({
    clientId: 'profile',
    taxId: '20123456786',
    name: 'Customer',
    ownershipUnproved: false,
    contacts: [{ kind: 'EMAIL', value: 'customer@example.test', primary: true, verified: false }],
  });
  render(await OrdersPage({ searchParams: Promise.resolve({}) }));
  expect(screen.getByText(messages.customerOrders.emptyUnverified)).toBeDefined();
  expect(screen.getByText(messages.customerOrders.verifyContactHint)).toBeDefined();
  expect(screen.queryByText(messages.customerOrders.proveEarlierContact)).toBeNull();
});

it('keeps the regular empty state after profile and contact verification', async () => {
  vi.mocked(getCustomerProfile).mockResolvedValue({
    clientId: 'profile',
    taxId: '20123456786',
    name: 'Customer',
    ownershipUnproved: false,
    contacts: [{ kind: 'EMAIL', value: 'customer@example.test', primary: true, verified: true }],
  });
  render(await OrdersPage({ searchParams: Promise.resolve({}) }));
  expect(screen.getByText(messages.customerOrders.empty)).toBeDefined();
  expect(screen.queryByRole('link', { name: messages.customerOrders.verifyContacts })).toBeNull();
});

it('loads existing orders without blocking them on the profile verification state', async () => {
  vi.mocked(getCustomerOrders).mockResolvedValue([
    {
      id: 'order',
      branchName: 'Branch',
      receivedAt: '2026-11-09T00:00:00Z',
      status: 'SENT',
      originalText: '',
      deliveryToken: 'delivery',
    },
  ]);
  render(await OrdersPage({ searchParams: Promise.resolve({}) }));
  expect(screen.getByRole('link', { name: /Branch/ }).getAttribute('href')).toBe('/orders/order');
  expect(getCustomerProfile).not.toHaveBeenCalled();
});
