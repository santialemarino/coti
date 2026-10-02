import { createElement } from 'react';
import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { makeQuotePayload } from '@/lib/api/public-quotes.fixtures';
import { getFormatters } from '@/lib/i18n/formatters-server';
import { QuoteHeader } from './quote-header';

vi.mock('next-intl/server', async () => import('@/lib/i18n/server-intl.mock'));

// `next/image` mirrors the intrinsic-dimension contract its call site leans on; a plain <img> is
// all jsdom needs to query the logo by its accessible name. createElement keeps the lint clean:
// the mock drops the Next sizing props the DOM would not understand.
vi.mock('next/image', () => ({
  default: ({
    alt,
    src,
    className,
  }: {
    alt: string;
    src: string;
    className?: string;
    fill?: boolean;
    unoptimized?: boolean;
  }) => createElement('img', { alt, src, className }),
}));

describe('QuoteHeader', () => {
  it('brands the quote with the supplier and lines up the stamp facts', async () => {
    const payload = makeQuotePayload();
    const fmt = await getFormatters();

    const view = render(
      await QuoteHeader({
        supplier: payload.supplier,
        branch: payload.branch,
        reference: payload.reference,
        versionNumber: payload.versionNumber,
        approvedAt: payload.approvedAt,
      }),
    );

    expect(view.getByText(payload.supplier.name)).toBeTruthy();
    expect(view.getByText(payload.branch.name)).toBeTruthy();
    expect(view.getByText(payload.branch.address ?? '')).toBeTruthy();
    expect(view.getByAltText(`Logo de ${payload.supplier.name}`)).toBeTruthy();
    expect(view.getByText(`Cotización ${payload.reference}`)).toBeTruthy();
    expect(view.getByText(`Versión ${payload.versionNumber}`)).toBeTruthy();
    expect(view.getByText(`Emitida el ${fmt.date(payload.approvedAt)}`)).toBeTruthy();
  });

  /*
   * The brand colour is tenant data, not a token, so it must reach the rule a caller could not
   * change: the only place it appears is that inline style, never a class that paints other text.
   */
  it('paints the tenant rule with the brand colour and nothing else', async () => {
    const payload = makeQuotePayload();

    const view = render(
      await QuoteHeader({
        supplier: payload.supplier,
        branch: payload.branch,
        reference: payload.reference,
        versionNumber: payload.versionNumber,
        approvedAt: payload.approvedAt,
      }),
    );

    expect(view.getByText('Corralón Centro').textContent).not.toContain(
      payload.supplier.brandColor,
    );
    const rule = view.container.querySelector('span[aria-hidden="true"]') as HTMLElement;
    // jsdom stores the inline colour normalised, so the assertion pins the rgb form of the fixture's hex.
    expect(rule.style.backgroundColor).toBe('rgb(194, 65, 12)');
  });

  it('renders no logo when the account has not uploaded one', async () => {
    const payload = makeQuotePayload({
      supplier: { ...makeQuotePayload().supplier, logoUrl: undefined },
    });

    const view = render(
      await QuoteHeader({
        supplier: payload.supplier,
        branch: payload.branch,
        reference: payload.reference,
        versionNumber: payload.versionNumber,
        approvedAt: payload.approvedAt,
      }),
    );

    expect(view.queryByAltText(/Logo de/)).toBeNull();
  });
});
