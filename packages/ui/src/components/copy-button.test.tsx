import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { CopyButton } from './copy-button';

const LABELS = { copy: 'Copiar enlace', copied: 'Enlace copiado' };
const URL = 'https://coti.test/q/abc';

function announced() {
  return screen.getByRole('status').textContent;
}

async function click() {
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: LABELS.copy }));
  });
}

describe('CopyButton', () => {
  const writeText = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers();
    writeText.mockReset().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  });
  afterEach(() => vi.useRealTimers());

  it('copies the value and confirms it', async () => {
    render(<CopyButton value={URL} labels={LABELS} />);
    await click();

    expect(writeText).toHaveBeenCalledWith(URL);
    expect(announced()).toBe(LABELS.copied);
  });

  it('offers to copy again once the confirmation has had its time', async () => {
    render(<CopyButton value={URL} labels={LABELS} />);
    await click();
    act(() => vi.advanceTimersByTime(2000));

    expect(announced()).toBe('');
  });

  // A second click restarts the wait, so the confirmation cannot vanish right after it.
  it('restarts the wait on another click', async () => {
    render(<CopyButton value={URL} labels={LABELS} />);
    await click();
    act(() => vi.advanceTimersByTime(1500));
    await click();
    act(() => vi.advanceTimersByTime(1000));

    expect(announced()).toBe(LABELS.copied);
  });

  it('reports a refused write instead of confirming it', async () => {
    const onCopyError = vi.fn();
    writeText.mockRejectedValue(new Error('denied'));
    render(<CopyButton value={URL} labels={LABELS} onCopyError={onCopyError} />);
    await click();

    expect(onCopyError).toHaveBeenCalledOnce();
    expect(announced()).toBe('');
  });
});
