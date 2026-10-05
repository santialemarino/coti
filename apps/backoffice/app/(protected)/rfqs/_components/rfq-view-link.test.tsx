import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import { ROUTES } from '@/config/routes';
import messages from '@/translations/es.json';

const copy = messages.rfqs.view;

function renderLink(to: 'queue' | 'table') {
  render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <RfqViewLink to={to} />
    </NextIntlClientProvider>,
  );
}

describe('RfqViewLink', () => {
  // A real link, so the other view opens in a new tab as well as in place.
  it('leads from the queue to the table', () => {
    renderLink('table');

    expect(screen.getByRole('link', { name: copy.toTable }).getAttribute('href')).toBe(ROUTES.rfqs);
  });

  it('leads from the table back to the queue', () => {
    renderLink('queue');

    expect(screen.getByRole('link', { name: copy.toQueue }).getAttribute('href')).toBe(ROUTES.home);
  });
});
