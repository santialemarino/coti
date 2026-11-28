import { fireEvent, render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { SplitOrderCallout } from '@/app/(protected)/rfqs/[id]/_components/split-order-callout';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const NOW = Date.parse('2026-11-28T12:00:00.000Z');

function renderCallout(splitAt: string, createdAt: string, onArchive = vi.fn()) {
  render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <SplitOrderCallout
        split={{ rfq_id: 'r1', quote_number: 41, created_at: splitAt }}
        createdAt={createdAt}
        archiving={false}
        onArchive={onArchive}
      />
    </NextIntlClientProvider>,
  );
  return onArchive;
}

beforeEach(() => vi.spyOn(Date, 'now').mockReturnValue(NOW));
afterEach(() => vi.restoreAllMocks());

describe('SplitOrderCallout', () => {
  it('says an earlier order from the client arrived minutes ago and links to it', () => {
    renderCallout('2026-11-28T11:48:00.000Z', '2026-11-28T11:55:00.000Z');

    expect(screen.getByText('Otro pedido de este cliente hace 12 min')).toBeTruthy();
    const link = screen.getByRole('link');
    expect(link.getAttribute('href')).toBe(ROUTES.rfqsDetail('r1'));
    expect(link.textContent).toContain('41');
  });

  it('says the client sent another order after this one', () => {
    renderCallout('2026-11-28T11:58:00.000Z', '2026-11-28T11:50:00.000Z');

    expect(screen.getByText('Este cliente mandó otro pedido hace 2 min')).toBeTruthy();
  });

  it('archives this order from the callout', () => {
    const onArchive = renderCallout('2026-11-28T11:48:00.000Z', '2026-11-28T11:55:00.000Z');

    fireEvent.click(
      screen.getByRole('button', { name: messages.rfqs.detail.callouts.splitOrder.archive }),
    );
    expect(onArchive).toHaveBeenCalledOnce();
  });
});
