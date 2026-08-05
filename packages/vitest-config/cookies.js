import { vi } from 'vitest';

/*
 * A stand-in for Next's cookie store, faithful on the one behaviour a hand-rolled fake gets
 * wrong: `delete` is `set(name, '')` with an expiry in the past, so the entry survives the
 * request carrying an empty value rather than disappearing. A jar that drops it instead is
 * kinder than production, and a reader that falls back with `??` then passes its test and
 * ships the bug — which is exactly how the branch switcher went blank after clearing.
 */
export function cookieJar(initial = {}) {
  const store = new Map(Object.entries(initial));

  return {
    get: vi.fn((name) => (store.has(name) ? { name, value: store.get(name) } : undefined)),
    set: vi.fn((name, value) => {
      store.set(name, value);
    }),
    delete: vi.fn((name) => {
      store.set(name, '');
    }),
  };
}
