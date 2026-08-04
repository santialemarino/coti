import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

// Testing Library only auto-cleans when the runner exposes a global afterEach, and these
// suites import their hooks explicitly. Without it a rendered tree survives into the next
// test and a query matches the previous one's DOM.
afterEach(cleanup);
