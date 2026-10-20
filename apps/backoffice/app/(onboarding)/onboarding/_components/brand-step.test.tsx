import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { BrandStep } from '@/app/(onboarding)/onboarding/_components/brand-step';
import type { Account } from '@/lib/api/account';
import messages from '@/translations/es.json';

function renderStep(taxId: string | null) {
  const account: Account = {
    id: 'a1',
    name: 'Corralón San Martín',
    legalName: 'Corralón San Martín S.R.L.',
    taxId,
    brandLogoUrl: null,
    brandColor: null,
    preferredLowerPriceStrategy: 'FIND_CHEAPER_ALTERNATIVES',
    invoicingEnabled: false,
    hasInvoices: false,
  };
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <BrandStep account={account} formId="brand-step" onSubmit={() => {}} />
    </NextIntlClientProvider>,
  );
}

describe('BrandStep', () => {
  it('shows a stored CUIT with its separators', () => {
    renderStep('30712345671');

    expect(screen.getByText('Corralón San Martín S.R.L. · 30-71234567-1')).toBeTruthy();
  });

  // An account registered before CUITs were checked may hold text with no digits at all.
  it('shows a tax id with no digits as it was written', () => {
    renderStep('en trámite');

    expect(screen.getByText('Corralón San Martín S.R.L. · en trámite')).toBeTruthy();
  });
});
