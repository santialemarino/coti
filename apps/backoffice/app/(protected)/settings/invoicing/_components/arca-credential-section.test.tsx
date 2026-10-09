import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { uploadArcaCredentials } from '@/app/(protected)/settings/invoicing/actions';
import messages from '@/translations/es.json';
import { ArcaCredentialSection } from './arca-credential-section';

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock('next/navigation', () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock('@/app/(protected)/settings/invoicing/actions', () => ({
  uploadArcaCredentials: vi.fn(),
  deleteArcaCredentials: vi.fn(),
}));

const copy = messages.invoicing.settings.credential;

function renderSection() {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <ArcaCredentialSection credential={null} />
    </NextIntlClientProvider>,
  );
}

function choose(view: ReturnType<typeof renderSection>, certificate: File, key: File) {
  const [certificateInput, keyInput] = view.container.querySelectorAll('input[type="file"]');
  fireEvent.change(certificateInput!, { target: { files: [certificate] } });
  fireEvent.change(keyInput!, { target: { files: [key] } });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('ArcaCredentialSection', () => {
  it('asks for both files before uploading anything', () => {
    const view = renderSection();

    fireEvent.click(view.getByRole('button', { name: copy.upload }));

    expect(view.getByText(copy.bothRequired)).toBeTruthy();
    expect(uploadArcaCredentials).not.toHaveBeenCalled();
  });

  // Next refuses a body past its limit before the action runs, which would surface as a crash.
  it('refuses a pair too large to be a certificate without sending it', () => {
    const view = renderSection();
    choose(view, new File(['x'.repeat(1024 * 1024)], 'cert.crt'), new File(['k'], 'key.key'));

    fireEvent.click(view.getByRole('button', { name: copy.upload }));

    expect(view.getByText(copy.errors.FILE_TOO_LARGE)).toBeTruthy();
    expect(uploadArcaCredentials).not.toHaveBeenCalled();
  });

  it('keeps the page and says so when the upload itself fails', async () => {
    vi.mocked(uploadArcaCredentials).mockRejectedValue(new Error('network'));
    const view = renderSection();
    choose(view, new File(['cert'], 'cert.crt'), new File(['key'], 'key.key'));

    fireEvent.click(view.getByRole('button', { name: copy.upload }));

    await waitFor(() => expect(view.getByText(messages.errors.INTERNAL)).toBeTruthy());
  });
});
