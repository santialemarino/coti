import { render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';

vi.mock('next-intl/server', () => ({ getTranslations: vi.fn() }));
vi.mock('@/lib/i18n/formatters-server', () => ({
  getFormatters: vi.fn(async () => ({ date: (iso: string) => `date:${iso}` })),
}));

const { getTranslations } = await import('next-intl/server');
const { LegalPage } = await import('@/app/(public)/_components/legal-page');

function lookup(path: string) {
  return path
    .split('.')
    .reduce<unknown>((node, segment) => (node as Record<string, unknown>)?.[segment], messages);
}

// Resolves against the real catalog, interpolating the one placeholder this page uses.
function translator(namespace: string) {
  const t = (key: string, values?: Record<string, string>) =>
    String(lookup(`${namespace}.${key}`)).replace('{date}', values?.date ?? '');
  return Object.assign(t, { raw: (key: string) => lookup(`${namespace}.${key}`) });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getTranslations).mockImplementation((async (namespace: string) =>
    translator(namespace)) as never);
});

describe('LegalPage', () => {
  it.each(['privacy', 'terms'] as const)('renders every section of the %s text', async (key) => {
    const copy = messages.legal[key];
    const view = render(await LegalPage({ namespace: `legal.${key}`, updatedAt: '2026-10-09' }));

    expect(view.getByRole('heading', { level: 1, name: copy.title })).toBeTruthy();
    expect(view.getAllByRole('heading', { level: 2 })).toHaveLength(copy.sections.length);
    expect(view.getByText(/date:2026-10-09/)).toBeTruthy();
  });

  // Ley 25.326 requires the control authority's notice on a privacy policy published in Argentina.
  it('names the data protection authority in the privacy policy', () => {
    const text = messages.legal.privacy.sections.flatMap((section) => section.paragraphs).join(' ');

    expect(text).toContain('Agencia de Acceso a la Información Pública');
  });
});
