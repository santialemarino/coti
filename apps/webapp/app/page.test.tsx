import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import HomePage from './page';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));
vi.mock('@/components/brand', () => ({
  Brand: () => <div data-testid="brand" />,
}));

describe('HomePage', () => {
  it('renders a centered terminal state instead of an endless loading message', async () => {
    const view = render(await HomePage());
    const main = view.getByRole('main');

    expect(main.className).toContain('min-h-dvh');
    expect(main.className).toContain('place-items-center');
    expect(view.getByText('No hay una cotización abierta')).toBeTruthy();
    expect(view.queryByText('Cargando…')).toBeNull();
  });
});
