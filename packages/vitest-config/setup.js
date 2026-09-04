import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

/*
 * jsdom implements no ResizeObserver, and Radix measures with one the moment a Checkbox, Switch
 * or RadioGroup mounts — so any component test rendering a form dies without this. A stub is
 * enough: nothing under test asserts on a measurement.
 */
if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// Testing Library only auto-cleans when the runner exposes a global afterEach, and these
// suites import their hooks explicitly. Without it a rendered tree survives into the next
// test and a query matches the previous one's DOM.
afterEach(cleanup);
