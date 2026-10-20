import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { Dropzone } from './dropzone';

describe('Dropzone accessibility', () => {
  it('exposes one file-selection control and carries the form error relationship', () => {
    const view = render(
      <Dropzone
        id="order-file"
        accept="text/plain"
        onFile={vi.fn()}
        title="Upload"
        releaseLabel="Drop"
        chooseLabel="Choose"
        aria-invalid="true"
        aria-describedby="file-error"
      />,
    );
    const button = view.getByRole('button');
    expect(button.id).toBe('order-file');
    expect(button.getAttribute('aria-invalid')).toBe('true');
    expect(button.getAttribute('aria-describedby')).toBe('file-error');
    expect(view.container.querySelector('input')?.tabIndex).toBe(-1);
    expect(view.container.querySelector('input')?.getAttribute('aria-hidden')).toBe('true');
  });
});
