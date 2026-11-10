import { afterEach, describe, expect, it, vi } from 'vitest';

import { showsTestEnvironment } from '@/lib/utils/invoicing-environment';

afterEach(() => vi.unstubAllEnvs());

describe('invoicing environment labels', () => {
  it('shows homologation labels during development only', () => {
    vi.stubEnv('NODE_ENV', 'development');
    expect(showsTestEnvironment('HOMOLOGATION')).toBe(true);
    expect(showsTestEnvironment('PRODUCTION')).toBe(false);
    expect(showsTestEnvironment(undefined)).toBe(false);
  });

  it('omits labels in production builds', () => {
    vi.stubEnv('NODE_ENV', 'production');
    expect(showsTestEnvironment('HOMOLOGATION')).toBe(false);
    expect(showsTestEnvironment('PRODUCTION')).toBe(false);
  });
});
