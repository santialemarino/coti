import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { LogoCropDialog } from '@/components/logo-crop-dialog';

const messages = (await import('@/translations/es.json')).default;
const copy = messages.common.logoUpload.crop;

beforeEach(() => {
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:crop');
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
  class LoadedImage {
    naturalWidth = 400;
    naturalHeight = 100;
    onload: (() => void) | null = null;
    set src(_value: string) {
      queueMicrotask(() => this.onload?.());
    }
  }
  vi.stubGlobal('Image', LoadedImage);
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
    clearRect: vi.fn(),
    drawImage: vi.fn(),
  } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) =>
    callback(new Blob(['cropped'], { type: 'image/png' })),
  );
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function setup() {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const view = render(
    <NextIntlClientProvider locale="es" messages={messages}>
      <LogoCropDialog
        file={new File(['source'], 'brand.jpg', { type: 'image/jpeg' })}
        onConfirm={onConfirm}
        onCancel={onCancel}
      />
    </NextIntlClientProvider>,
  );
  return { view, onConfirm, onCancel };
}

describe('LogoCropDialog', () => {
  it('exports the preview as a new PNG only after confirmation', async () => {
    const { view, onConfirm } = setup();
    await waitFor(() =>
      expect(view.getByRole('button', { name: copy.confirm }).hasAttribute('disabled')).toBe(false),
    );
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(view.getByRole('button', { name: copy.square }));
    fireEvent.change(view.getByLabelText(copy.zoom), { target: { value: '2' } });
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    const file = onConfirm.mock.calls[0]![0] as File;
    expect(file.name).toBe('brand.png');
    expect(file.type).toBe('image/png');
    expect(file.size).toBeGreaterThan(0);
  });
  it('cancels without replacing the stored logo', () => {
    const { view, onCancel, onConfirm } = setup();
    fireEvent.click(view.getByRole('button', { name: copy.cancel }));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
