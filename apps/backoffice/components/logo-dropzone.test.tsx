import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { LogoDropzone } from '@/components/logo-dropzone';

vi.mock('@/components/logo-crop-dialog', () => ({
  LogoCropDialog: ({
    open,
    file,
    onConfirm,
    onCancel,
  }: {
    open: boolean;
    file: File;
    onConfirm: (file: File) => void;
    onCancel: () => void;
  }) => (
    <div role="dialog" data-state={open ? 'open' : 'closed'}>
      <button onClick={() => onConfirm(file)}>Use crop</button>
      <button onClick={onCancel}>Cancel crop</button>
    </div>
  ),
}));
const messages = (await import('@/translations/es.json')).default;

afterEach(() => vi.unstubAllGlobals());

describe('LogoDropzone', () => {
  it('previews the selected image and reports it for upload', async () => {
    const createObjectURL = vi.fn(() => 'blob:local-logo');
    const onFileChange = vi.fn();
    const onPreviewChange = vi.fn();
    const NativeURL = URL;
    class LocalPreviewURL extends NativeURL {
      static createObjectURL = createObjectURL;
      static revokeObjectURL = vi.fn();
    }
    vi.stubGlobal('URL', LocalPreviewURL);
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <LogoDropzone onFileChange={onFileChange} onPreviewChange={onPreviewChange} />
      </NextIntlClientProvider>,
    );
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');
    const logo = new File(['logo'], 'logo.png', { type: 'image/png' });

    fireEvent.change(input!, { target: { files: [logo] } });

    expect(onFileChange).not.toHaveBeenCalled();
    fireEvent.click(view.getByRole('button', { name: 'Use crop' }));
    expect(createObjectURL).toHaveBeenCalledWith(logo);
    expect(input?.name).toBe('');
    expect(onFileChange).toHaveBeenCalledWith(logo);
    await waitFor(() => expect(onPreviewChange).toHaveBeenCalledWith('blob:local-logo'));
    expect(view.getByRole('img', { name: messages.common.logoUpload.previewAlt })).toBeTruthy();
  });

  // Unmounting the dialog the moment the crop lands would cut its exit short.
  it('keeps the crop dialog mounted, closed, while it fades out', () => {
    const view = render(
      <NextIntlClientProvider locale="es" messages={messages}>
        <LogoDropzone />
      </NextIntlClientProvider>,
    );
    const input = view.container.querySelector<HTMLInputElement>('input[type="file"]');

    fireEvent.change(input!, {
      target: { files: [new File(['logo'], 'logo.png', { type: 'image/png' })] },
    });
    expect(view.getByRole('dialog').dataset.state).toBe('open');
    fireEvent.click(view.getByRole('button', { name: 'Cancel crop' }));

    expect(view.getByRole('dialog').dataset.state).toBe('closed');
  });
});
