import { fireEvent, render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';
import { PublicRfqSiteForm } from './public-rfq-site-form';

vi.mock('@/app/rfq/[channelId]/_components/public-rfq-form', () => ({
  PublicRfqForm: ({ channelId }: { channelId: string }) => (
    <div data-testid="intake">{channelId}</div>
  ),
}));

it('requires a deliberate branch selection before opening that branch intake', async () => {
  const destination = {
    supplierName: 'Supplier',
    branchName: 'Centro',
    maxTextCharacters: 20000,
    maxFileSize: 1000,
    acceptedContentTypes: ['text/plain'],
  };
  const view = render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <PublicRfqSiteForm
        site={{
          supplierName: 'Supplier',
          branches: [
            { channelId: 'first', destination },
            { channelId: 'second', destination: { ...destination, branchName: 'Norte' } },
          ],
        }}
      />
    </NextIntlClientProvider>,
  );
  const submit = view.getByRole('button', { name: messages.rfq.branch.continue });
  expect(submit.hasAttribute('disabled')).toBe(true);
  expect(view.queryByTestId('intake')).toBeNull();
  fireEvent.click(view.getByRole('combobox'));
  fireEvent.click(view.getByRole('option', { name: 'Norte' }));
  fireEvent.click(submit);
  expect((await view.findByTestId('intake')).textContent).toBe('second');
});
