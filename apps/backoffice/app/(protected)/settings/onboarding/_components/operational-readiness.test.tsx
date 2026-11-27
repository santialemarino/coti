import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it } from 'vitest';

import { OperationalReadinessCard } from '@/app/(protected)/settings/onboarding/_components/operational-readiness';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

it('distinguishes empty catalog, missing prices and optional channels per branch', () => {
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <OperationalReadinessCard
        readiness={{
          accountReady: true,
          branches: [
            {
              id: 'one',
              name: 'Centro',
              products: 4,
              missingPrices: 2,
              hasStaff: true,
              hasChannel: false,
            },
            {
              id: 'two',
              name: 'Norte',
              products: 0,
              missingPrices: 0,
              hasStaff: true,
              hasChannel: true,
            },
          ],
        }}
      />
    </NextIntlClientProvider>,
  );
  expect(view.getByText('2 productos sin precio utilizable')).toBeTruthy();
  expect(view.getByText('Opcional')).toBeTruthy();
  expect(view.getAllByText('Pendiente')).toHaveLength(3);
  expect(view.getByRole('link', { name: 'Gestionar precios' }).getAttribute('href')).toBe(
    ROUTES.priceSettings,
  );
});
