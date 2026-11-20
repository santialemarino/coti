import { render, screen } from '@testing-library/react';
import { createTranslator } from 'next-intl';
import { expect, it, vi } from 'vitest';

import CustomerProfilePage from '@/app/profile/page';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({
  getTranslations: async () =>
    createTranslator({ locale: 'es-AR', messages, namespace: 'customerProfile' }),
}));
vi.mock('@/lib/auth/customer-flow', () => ({ requireCustomer: vi.fn() }));
vi.mock('@/components/customer-header', () => ({ CustomerHeader: () => null }));
vi.mock('@/components/customer-footer', () => ({ CustomerFooter: () => null }));
vi.mock('@/app/profile/_components/customer-profile-form', () => ({
  CustomerProfileForm: () => <form aria-label="profile" />,
}));
vi.mock('@/lib/api/customer-auth-options', () => ({
  customerPageMetadata: vi.fn(),
  getCustomerAuthOptions: async () => ({ supplierName: 'Corralón', passwordMinLength: 12 }),
}));
vi.mock('@/lib/api/customer-profile', () => ({
  getCustomerProfile: async () => ({
    clientId: 'profile',
    taxId: '20123456786',
    name: 'Customer',
    ownershipUnproved: false,
    contacts: [
      { kind: 'EMAIL', value: 'customer@example.test', primary: true, verified: true, main: true },
    ],
  }),
}));

it('shows the profile form and nothing after it', async () => {
  const { container } = render(await CustomerProfilePage());
  expect(screen.getByRole('heading', { name: messages.customerProfile.title })).toBeDefined();
  expect(screen.getByRole('form', { name: 'profile' })).toBeDefined();
  expect(container.querySelector('form')?.parentElement?.lastElementChild?.tagName).toBe('FORM');
});
