import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { EXPIRES_AT, makeQuotePayload, PDF_URL } from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { QuoteInfo } from './quote-info';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

describe('QuoteInfo', () => {
  it('states the validity and the seller note', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteInfo({
        expiresAt: EXPIRES_AT,
        validityNote: payload.validityNote,
        pdfUrl: PDF_URL,
      }),
    );

    expect(view.getByText('Válida hasta')).toBeTruthy();
    expect(view.getByText(fmt.date(EXPIRES_AT))).toBeTruthy();
    expect(view.getByText(payload.validityNote)).toBeTruthy();
    expect(view.getByText('Observaciones')).toBeTruthy();
  });

  it('links the PDF and opens it in a new tab', async () => {
    const payload = makeQuotePayload();

    const view = render(
      await QuoteInfo({
        expiresAt: EXPIRES_AT,
        validityNote: payload.validityNote,
        pdfUrl: PDF_URL,
      }),
    );

    const link = view.getByRole('link', { name: 'Descargar la cotización en PDF' });
    expect(link.getAttribute('href')).toBe(PDF_URL);
    expect(link.getAttribute('target')).toBe('_blank');
    expect(link.getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('skips the note and the PDF link when there are none', async () => {
    const fmt = await getFormatters();

    const view = render(
      await QuoteInfo({ expiresAt: EXPIRES_AT, validityNote: '', pdfUrl: undefined }),
    );

    expect(view.getByText('Válida hasta')).toBeTruthy();
    expect(view.getByText(fmt.date(EXPIRES_AT))).toBeTruthy();
    expect(view.queryByRole('link')).toBeNull();
  });
});
