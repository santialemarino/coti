import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it, vi } from 'vitest';

import { CustomerHeader } from '@/components/customer-header';
import { PortalProvider } from '@/components/portal-link';
import messages from '@/translations/es.json';

vi.mock('next/navigation', () => ({ usePathname: () => '/c/supplier/profile' }));
vi.mock('@/app/login/actions', () => ({ signOutCustomer: vi.fn() }));

it('keeps account navigation inside the selected supplier portal', () => {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <PortalProvider slug="supplier">
        <CustomerHeader section="profile" supplierName="Corralón Norte" />
      </PortalProvider>
    </NextIntlClientProvider>,
  );
  expect(view.getByRole('link', { name: 'Corralón Norte' }).getAttribute('href')).toBe(
    '/c/supplier/orders',
  );
  const profile = view.getByRole('link', { name: messages.customerOrders.profile });
  expect(profile.getAttribute('href')).toBe('/c/supplier/profile');
  expect(profile.getAttribute('aria-current')).toBe('page');
  expect(view.getByRole('link', { name: messages.customerOrders.title }).getAttribute('href')).toBe(
    '/c/supplier/orders',
  );
  expect(
    view.getByRole('button', { name: messages.customerOrders.logout }).getAttribute('type'),
  ).toBe('submit');
});
