import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import messages from '@/translations/es.json';
import { AcceptSentVersionButton } from './accept-sent-version-button';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('@/lib/api/rfqs-client', () => ({ acceptSentVersion: vi.fn() }));

const { acceptSentVersion } = await import('@/lib/api/rfqs-client');

const QUOTE_ID = '20000000-0000-4000-8000-000000000002';
const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';
const copy = messages.rfqs.detail.callouts.acceptSentVersion;

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(acceptSentVersion).mockResolvedValue({} as never);
});

describe('AcceptSentVersionButton', () => {
  // Accepting the sent version throws the open draft away, so the dialog says so before it runs.
  it('accepts the sent version only after a dialog naming what is discarded', async () => {
    const onChanged = vi.fn().mockResolvedValue(undefined);
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <AcceptSentVersionButton
          quoteId={QUOTE_ID}
          branchId={BRANCH_ID}
          reference="#07"
          sentVersion={2}
          onChanged={onChanged}
        />
      </NextIntlClientProvider>,
    );

    fireEvent.click(view.getByRole('button', { name: copy.button }));
    const dialog = view.getByRole('dialog');
    expect(dialog.textContent).toContain(copy.description.replace('{version}', '2'));
    expect(acceptSentVersion).not.toHaveBeenCalled();
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));

    await waitFor(() => expect(acceptSentVersion).toHaveBeenCalledWith(QUOTE_ID, BRANCH_ID));
    expect(onChanged).toHaveBeenCalled();
  });
});
