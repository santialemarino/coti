import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { RfqViewSwitch, type RfqView } from '@/app/(protected)/rfqs/_components/rfq-view-switch';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const router = vi.hoisted(() => ({ push: vi.fn(), prefetch: vi.fn() }));

vi.mock('next/navigation', () => ({ useRouter: () => router }));

const copy = messages.rfqs.view;

function renderSwitch(view: RfqView) {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqViewSwitch view={view} />
    </NextIntlClientProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe('RfqViewSwitch', () => {
  it('goes to the table from the queue, having fetched it ahead', () => {
    renderSwitch('queue');

    expect(router.prefetch).toHaveBeenCalledWith(ROUTES.rfqs);
    fireEvent.click(screen.getByRole('radio', { name: copy.table }));
    expect(router.push).toHaveBeenCalledWith(ROUTES.rfqs);
  });

  it('goes back to the queue from the table', () => {
    renderSwitch('table');

    fireEvent.click(screen.getByRole('radio', { name: copy.queue }));
    expect(router.push).toHaveBeenCalledWith(ROUTES.home);
  });

  // Pressing the lit segment would clear a single toggle group and leave no view chosen.
  it('ignores a press on the view already showing', () => {
    renderSwitch('queue');

    fireEvent.click(screen.getByRole('radio', { name: copy.queue }));
    expect(router.push).not.toHaveBeenCalled();
    expect(screen.getByRole('radio', { name: copy.queue }).getAttribute('aria-checked')).toBe(
      'true',
    );
  });
});
