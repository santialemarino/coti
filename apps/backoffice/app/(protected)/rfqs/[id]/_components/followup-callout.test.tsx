import { render, screen } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it, vi } from 'vitest';

import { FollowupCallout } from '@/app/(protected)/rfqs/[id]/_components/followup-callout';
import type { RfqDetailResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

vi.mock('@/app/(protected)/rfqs/[id]/_components/send-quote-dialog', () => ({
  SendQuoteDialog: () => <button>Send follow-up</button>,
}));
describe('follow-up callout', () => {
  it('offers the shared confirmation flow when the customer is expected to answer', () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <FollowupCallout
          quoteId="quote"
          branchId="branch"
          origin="customer"
          quietSince={null}
          detail={{} as RfqDetailResponse}
          onSent={vi.fn()}
        />
      </NextIntlClientProvider>,
    );
    expect(screen.getByRole('button', { name: 'Send follow-up' })).toBeTruthy();
    expect(screen.queryByRole('textbox')).toBeNull();
  });
  it('does not offer customer contact for work still waiting on the seller', () => {
    render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <FollowupCallout quoteId="quote" branchId="branch" origin="seller" quietSince={null} />
      </NextIntlClientProvider>,
    );
    expect(screen.queryByRole('button')).toBeNull();
  });
});
