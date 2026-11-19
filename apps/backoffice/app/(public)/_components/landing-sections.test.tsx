import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/lib/i18n/formatters-server', async () => {
  const { createFormatters } = await import('@/lib/i18n/create-formatters');
  return { getFormatters: async () => createFormatters('es-AR', 'America/Argentina/Buenos_Aires') };
});

const { getTranslations } = await import('next-intl/server');
const { LandingBenefits } = await import('@/app/(public)/_components/landing-benefits');
const { LandingChanges } = await import('@/app/(public)/_components/landing-changes');
const { LandingFaq } = await import('@/app/(public)/_components/landing-faq');
const { LandingFeatures } = await import('@/app/(public)/_components/landing-features');
const { LandingFit } = await import('@/app/(public)/_components/landing-fit');
const { LandingSteps } = await import('@/app/(public)/_components/landing-steps');
const { LandingTrust } = await import('@/app/(public)/_components/landing-trust');

// A missing key falls back to the key itself, which is what next-intl would put on the page.
function translator(namespace: string) {
  return (key: string) => {
    const value = `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
    return typeof value === 'string' ? value : `${namespace}.${key}`;
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('landing sections', () => {
  it.each([
    ['fit', LandingFit],
    ['benefits', LandingBenefits],
    ['steps', LandingSteps],
    ['changes', LandingChanges],
    ['features', LandingFeatures],
    ['trust', LandingTrust],
    ['faq', LandingFaq],
  ] as const)('renders every %s entry from the catalog', async (_, Section) => {
    const view = render(await Section());

    expect(view.container.textContent).not.toContain('landing.');
    expect(view.getByRole('heading', { level: 2 })).toBeTruthy();
  });

  // Past three, a list of benefits stops helping anyone decide.
  it('keeps the headline benefits to three', async () => {
    const view = render(await LandingBenefits());

    expect(view.getAllByRole('heading', { level: 3 })).toHaveLength(3);
  });

  // The «está caro» example has to add up: the new total is the old one less the cheaper cement.
  it('shows the change example lowering the total by exactly the swapped line', async () => {
    const view = render(await LandingChanges());

    expect(view.getByText('$ 409.000,00').tagName).toBe('S');
    expect(view.getByText('$ 382.000,00')).toBeTruthy();
  });
});
