import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { useHeldWhileClosed } from './use-held-while-closed';

describe('useHeldWhileClosed', () => {
  it('passes the value through while open', () => {
    const { result, rerender } = renderHook(({ value, open }) => useHeldWhileClosed(value, open), {
      initialProps: { value: 'a', open: true },
    });
    rerender({ value: 'b', open: true });

    expect(result.current).toBe('b');
  });

  it('holds the last open value once closed, whatever the caller passes', () => {
    const { result, rerender } = renderHook(({ value, open }) => useHeldWhileClosed(value, open), {
      initialProps: { value: 'row', open: true },
    });
    rerender({ value: 'reset', open: false });
    rerender({ value: 'another reset', open: false });

    expect(result.current).toBe('row');
  });

  it('follows the caller again on the next open', () => {
    const { result, rerender } = renderHook(({ value, open }) => useHeldWhileClosed(value, open), {
      initialProps: { value: 'first', open: true },
    });
    rerender({ value: 'cleared', open: false });
    rerender({ value: 'second', open: true });

    expect(result.current).toBe('second');
  });
});
