import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { PublicRequestCard } from '@/app/(protected)/rfqs/[id]/_components/public-request-card';
import { mapRfqDetail, type RfqDetailResponse } from '@/lib/api/rfqs';
import messages from '@/translations/es.json';

describe('public request declarations', () => {
  it('maps the new wire fields before rendering contact and the original order', () => {
    const detail = mapRfqDetail({
      public_contact: { tax_id: '20123456786', name: 'Obra Norte', email: 'contacto@example.com' },
      public_original_text: '10 bolsas de cemento',
    } as Parameters<typeof mapRfqDetail>[0]);
    expect(detail.publicRequest?.taxId).toBe('20123456786');
    expect((detail as unknown as Record<string, unknown>).public_contact).toBeUndefined();
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <PublicRequestCard request={detail.publicRequest!} />
      </NextIntlClientProvider>,
    );
    expect(view.getByText('contacto@example.com')).toBeTruthy();
    expect(view.getByText('10 bolsas de cemento')).toBeTruthy();
    expect(view.queryByText(messages.rfqs.detail.publicRequest.phone)).toBeNull();
  });

  it('leaves existing RFQ details without a public declaration block', () => {
    expect(mapRfqDetail({ rfq: {} } as RfqDetailResponse).publicRequest).toBeUndefined();
  });
});
