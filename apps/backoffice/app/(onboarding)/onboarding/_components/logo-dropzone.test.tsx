import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { LogoDropzone } from '@/app/(onboarding)/onboarding/_components/logo-dropzone';

const messages = (await import('@/translations/es.json')).default;

afterEach(() => vi.unstubAllGlobals());

describe('LogoDropzone', () => {
  it('previews the selected image locally without adding it to form data', async () => {
    const createObjectURL = vi.fn(() => 'blob:local-logo');
    const onPreviewChange = vi.fn();
    const NativeURL = URL;
    class LocalPreviewURL extends NativeURL {
      static createObjectURL = createObjectURL;
      static revokeObjectURL = vi.fn();
    }
    vi.stubGlobal('URL', LocalPreviewURL);
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <LogoDropzone onPreviewChange={onPreviewChange} />
      </NextIntlClientProvider>,
    );
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');
    const logo = new File(['logo'], 'logo.png', { type: 'image/png' });

    fireEvent.change(input!, { target: { files: [logo] } });

    expect(createObjectURL).toHaveBeenCalledWith(logo);
    expect(input?.name).toBe('');
    expect(view.getByText(messages.onboarding.brand.logo.localOnly)).toBeTruthy();
    await waitFor(() => expect(onPreviewChange).toHaveBeenCalledWith('blob:local-logo'));
    expect(view.getByRole('img', { name: messages.onboarding.brand.logo.previewAlt })).toBeTruthy();
  });
});
