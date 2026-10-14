import { fireEvent, render, waitFor } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { LogoCropDialog } from '@/components/logo-crop-dialog';

const messages = (await import('@/translations/es.json')).default;
const copy = messages.common.logoUpload.crop;

const drawImage = vi.fn();

beforeEach(() => {
  drawImage.mockClear();
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
    drawImage,
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
        open
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
    fireEvent.click(view.getByRole('radio', { name: copy.square }));
    fireEvent.change(view.getByLabelText(copy.zoom), { target: { value: '2' } });
    fireEvent.click(view.getByRole('button', { name: copy.confirm }));
    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    const file = onConfirm.mock.calls[0]![0] as File;
    expect(file.name).toBe('brand.png');
    expect(file.type).toBe('image/png');
    expect(file.size).toBeGreaterThan(0);
  });
  /*
   * The 400×100 image fills the 600×150 frame; at zoom 2 it is 1200 wide and pans 300px either way.
   * Pressed far past the edge and back once, it must move at once: 0.5 − 0.03 of the frame is 282px,
   * so the image starts at −300 + 282 = −18. Unclamped, the offset would still sit past the edge.
   */
  it('pans back from an edge without a dead zone', async () => {
    const { view } = setup();
    await waitFor(() => expect(drawImage).toHaveBeenCalled());
    fireEvent.change(view.getByLabelText(copy.zoom), { target: { value: '2' } });
    const preview = view.getByRole('img', { name: copy.preview });

    for (let press = 0; press < 30; press += 1) fireEvent.keyDown(preview, { key: 'ArrowRight' });
    fireEvent.keyDown(preview, { key: 'ArrowLeft' });

    await waitFor(() => expect(drawImage.mock.lastCall?.[1]).toBeCloseTo(-18));
  });

  it('cancels without replacing the stored logo', () => {
    const { view, onCancel, onConfirm } = setup();
    fireEvent.click(view.getByRole('button', { name: copy.cancel }));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
