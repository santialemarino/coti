import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));

const { getTranslations } = await import('next-intl/server');
const { LandingBenefits } = await import('@/app/(public)/_components/landing-benefits');
const { LandingFaq } = await import('@/app/(public)/_components/landing-faq');
const { LandingFeatures } = await import('@/app/(public)/_components/landing-features');
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
    ['benefits', LandingBenefits],
    ['steps', LandingSteps],
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
});
