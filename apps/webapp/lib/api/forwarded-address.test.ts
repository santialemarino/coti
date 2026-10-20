import { describe, expect, it } from 'vitest';

import { forwardedClientAddress } from '@/lib/api/forwarded-address';

describe('public intake proxy identity', () => {
  it('ignores untrusted chains and selects only the configured hop', () => {
    const headers = new Headers({ 'X-Forwarded-For': '203.0.113.9, 198.51.100.4' });
    expect(forwardedClientAddress(headers, 0)).toBeUndefined();
    expect(forwardedClientAddress(headers, 1)).toBe('198.51.100.4');
    expect(forwardedClientAddress(headers, 3)).toBeUndefined();
    expect(
      forwardedClientAddress(new Headers({ 'X-Forwarded-For': 'invalid' }), 1),
    ).toBeUndefined();
  });
});
