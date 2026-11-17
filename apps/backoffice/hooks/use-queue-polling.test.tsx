import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useQueuePolling } from '@/hooks/use-queue-polling';
import { RFQ_REFRESH_INTERVAL_SECONDS } from '@/lib/config';

afterEach(() => vi.useRealTimers());

describe('queue polling', () => {
  it('retries a failed read using the latest callback without overlapping requests', async () => {
    vi.useFakeTimers();
    const first = vi.fn().mockRejectedValue(new Error('Offline'));
    const next = vi.fn().mockResolvedValue(undefined);
    const hook = renderHook(({ refresh }) => useQueuePolling(refresh, true), {
      initialProps: { refresh: first },
    });
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 1000));
    expect(first).toHaveBeenCalledTimes(1);
    hook.rerender({ refresh: next });
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 1000));
    expect(next).toHaveBeenCalledTimes(1);
    hook.unmount();
  });
  it('waits for the previous read and stops after leaving the screen', async () => {
    vi.useFakeTimers();
    let finish!: () => void;
    const refresh = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const hook = renderHook(() => useQueuePolling(refresh, true));
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 1000));
    expect(refresh).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 3000));
    expect(refresh).toHaveBeenCalledTimes(1);
    await act(async () => finish());
    hook.unmount();
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 1000));
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('does not read while hidden and refreshes when the seller returns', async () => {
    vi.useFakeTimers();
    const state = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    const refresh = vi.fn().mockResolvedValue(undefined);
    const hook = renderHook(() => useQueuePolling(refresh, true));
    await act(() => vi.advanceTimersByTimeAsync(RFQ_REFRESH_INTERVAL_SECONDS * 1000));
    expect(refresh).not.toHaveBeenCalled();
    state.mockReturnValue('visible');
    await act(async () => {
      document.dispatchEvent(new Event('visibilitychange'));
    });
    expect(refresh).toHaveBeenCalledTimes(1);
    hook.unmount();
    state.mockRestore();
  });
});
