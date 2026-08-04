import { describe, expect, it } from 'vitest';

import { ROUTES, safeNextPath } from '@/config/routes';

describe('safeNextPath', () => {
  it('keeps a same-origin path with its query', () => {
    expect(safeNextPath('/settings/prices?branch=1')).toBe('/settings/prices?branch=1');
  });

  it.each([undefined, null, ''])('falls back home for %p', (raw) => {
    expect(safeNextPath(raw)).toBe(ROUTES.home);
  });

  /*
   * The reason this function exists rather than a startsWith('/') check: the URL parser reads a
   * backslash as a slash, so each of these resolves to an off-origin host while still looking
   * like a relative path.
   */
  it.each(['/\\evil.com', '/\\/evil.com', '//evil.com', '/\\\\evil.com'])(
    'refuses the backslash-authority form %p',
    (raw) => {
      expect(safeNextPath(raw)).toBe(ROUTES.home);
    },
  );

  it('refuses an absolute URL on another origin', () => {
    expect(safeNextPath('https://evil.com/steal')).toBe(ROUTES.home);
  });

  it('drops the fragment, which the server never receives anyway', () => {
    expect(safeNextPath('/settings#section')).toBe('/settings');
  });
});
