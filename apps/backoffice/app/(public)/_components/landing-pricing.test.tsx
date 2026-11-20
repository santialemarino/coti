import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/lib/i18n/formatters-server', async () => {
  const { createFormatters } = await import('@/lib/i18n/create-formatters');
  return { getFormatters: async () => createFormatters('es-AR', 'America/Argentina/Buenos_Aires') };
});
// An async server component of its own, which a client render cannot nest; landing-cta.test covers it.
vi.mock('@/app/(public)/_components/landing-cta', () => ({
  LandingCta: ({ signupEnabled }: { signupEnabled: boolean }) => (
    <div data-testid="cta" data-signup={String(signupEnabled)} />
  ),
}));

const { getTranslations } = await import('next-intl/server');
const { LandingPricing } = await import('@/app/(public)/_components/landing-pricing');

// Resolves against the real catalog with ICU arguments, so a missing key shows up as the key.
function translator(namespace: string) {
  return (key: string, values: Record<string, string | number> = {}) => {
    const found = `${namespace}.${key}`
      .split('.')
      .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
    if (typeof found !== 'string') return `${namespace}.${key}`;
    return found
      .replace(/\{count, plural, one \{# (\S+)\} other \{# (\S+)\}\}/, (_, one, other) =>
        values.count === 1 ? `1 ${one}` : `${values.count} ${other}`,
      )
      .replace(/\{(\w+)\}/g, (_, name: string) => String(values[name]));
  };
}

// Currency output carries non-breaking spaces, so the text is compared with plain ones.
function text(view: ReturnType<typeof render>) {
  return (view.container.textContent ?? '').replace(/\s/g, ' ');
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('LandingPricing', () => {
  it('is the section the header scrolls to', async () => {
    const view = render(await LandingPricing({ signedIn: false, signupEnabled: false }));

    expect(view.container.querySelector('section')?.id).toBe('probar');
    expect(ROUTES.landingSection('probar')).toBe('/#probar');
  });

  // The figures are the owner's, typed by hand here: a drifted constant must fail, not re-derive.
  it('lists the three plans with their monthly price, quotations and branches', async () => {
    const view = render(await LandingPricing({ signedIn: false, signupEnabled: false }));
    const plans = view.getAllByRole('listitem').filter((item) => item.querySelector('h3'));
    const [uno, pro, red] = plans.map((plan) => (plan.textContent ?? '').replace(/\s/g, ' '));

    expect(plans).toHaveLength(3);
    expect(uno).toBe(
      'UnoPara empezar con una sucursal.$ 150.000por mes300 cotizaciones por mes1 sucursal',
    );
    expect(pro).toBe(
      'ProPara una sucursal con mucho movimiento.$ 270.000por mes900 cotizaciones por mes1 sucursal',
    );
    expect(red).toBe(
      'RedPara corralones con más de una sucursal.$ 390.000por mes' +
        '1.500 cotizaciones por mes, compartidas2 sucursales' +
        '+$ 120.000 y +750 cotizaciones por cada sucursal adicional',
    );
    expect(text(view)).toContain(
      'Cotización adicional: $ 900, en los tres planes. Onboarding incluido en todos los planes.',
    );
  });

  it('says nothing the owner kept off the page', async () => {
    const view = render(await LandingPricing({ signedIn: false, signupEnabled: true }));

    expect(text(view)).not.toMatch(/fundador|IVA|anual|landing\./i);
  });

  it('closes on the call to action for a visitor, following the switch', async () => {
    const view = render(await LandingPricing({ signedIn: false, signupEnabled: false }));

    expect(view.getByTestId('cta').dataset.signup).toBe('false');
  });

  it('shows a seller the plans without the call to action', async () => {
    const view = render(await LandingPricing({ signedIn: true, signupEnabled: true }));

    expect(view.getAllByRole('heading', { level: 3 })).toHaveLength(3);
    expect(view.queryByTestId('cta')).toBeNull();
  });
});
