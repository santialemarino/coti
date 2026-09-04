import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { OnboardingProgress } from '@/app/(onboarding)/onboarding/_components/onboarding-progress';

const messages = (await import('@/translations/es.json')).default;

describe('OnboardingProgress', () => {
  it('keeps step statuses accessible without rendering labels below the bars', () => {
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <OnboardingProgress current="BRAND" resolved={{ WELCOME: 'COMPLETED' }} />
      </NextIntlClientProvider>,
    );

    expect(view.getByText(/Bienvenida: Listo/).classList.contains('sr-only')).toBe(true);
    expect(view.getByText(/Marca: En curso/).classList.contains('sr-only')).toBe(true);
    expect(view.queryByText(/Paso 2 de 7/)).toBeNull();
    expect(view.container.querySelectorAll('ol > li > span:not(.sr-only)')).toHaveLength(7);
  });

  it('keeps completed, current, and pending bars contiguous after navigating backwards', () => {
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <OnboardingProgress
          current="BRAND"
          resolved={{
            WELCOME: 'COMPLETED',
            BRAND: 'COMPLETED',
            FIRST_BRANCH: 'COMPLETED',
          }}
        />
      </NextIntlClientProvider>,
    );

    const states = [...view.container.querySelectorAll('[data-state]')].map((bar) =>
      bar.getAttribute('data-state'),
    );

    expect(states).toEqual([
      'completed',
      'current',
      'pending',
      'pending',
      'pending',
      'pending',
      'pending',
    ]);
    expect(view.getByText(/Sucursal: Pendiente/).classList.contains('sr-only')).toBe(true);
  });
});
