import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { needsRenewal, sessionCookieOptions } from '@/lib/auth/tokens';

const NOW_SECONDS = 1_800_000_000;
// The default AUTH_REFRESH_SKEW_SECONDS, which is what these cases are measured against.
const SKEW_SECONDS = 60;

// A token is only ever read for its unverified exp claim, so a signature is not needed to
// exercise the one thing the backoffice does with it.
function tokenExpiringAt(epochSeconds: number): string {
  const payload = Buffer.from(JSON.stringify({ exp: epochSeconds })).toString('base64url');
  return `header.${payload}.signature`;
}

describe('needsRenewal', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(NOW_SECONDS * 1000);
  });

  afterEach(() => vi.useRealTimers());

  it('renews when there is no token at all', () => {
    expect(needsRenewal(undefined)).toBe(true);
  });

  it('holds a token with room to spare', () => {
    expect(needsRenewal(tokenExpiringAt(NOW_SECONDS + SKEW_SECONDS + 1))).toBe(false);
  });

  // The skew exists so a request cannot start with a token that expires mid-flight, which
  // means the boundary itself has to renew rather than pass.
  it('renews exactly at the skew boundary', () => {
    expect(needsRenewal(tokenExpiringAt(NOW_SECONDS + SKEW_SECONDS))).toBe(true);
  });

  it('renews an already-expired token', () => {
    expect(needsRenewal(tokenExpiringAt(NOW_SECONDS - 1))).toBe(true);
  });

  it.each([
    ['not a jwt', 'garbage'],
    ['no payload segment', 'header'],
    ['payload that is not base64 json', 'header.@@@@.signature'],
    ['payload with no exp', `header.${Buffer.from('{}').toString('base64url')}.signature`],
    ['exp that is not a number', `header.${Buffer.from('{"exp":"soon"}').toString('base64url')}.s`],
  ])('renews rather than trusting a token with %s', (_label, token) => {
    expect(needsRenewal(token)).toBe(true);
  });
});

/*
 * TRUSTED_PROXY_HOPS is read once at module load, so each hop count needs a fresh module
 * graph rather than a mutated constant.
 */
async function forwardedAddressWithHops(hops: number) {
  vi.stubEnv('WEB_TRUSTED_PROXY_HOPS', String(hops));
  vi.resetModules();
  const { forwardedClientAddress } = await import('@/lib/auth/tokens');
  return forwardedClientAddress;
}

describe('forwardedClientAddress', () => {
  afterEach(() => vi.unstubAllEnvs());

  // The local case: nothing sits in front, so any header value was written by the caller.
  it('trusts nothing when no hop is declared', async () => {
    const resolve = await forwardedAddressWithHops(0);
    expect(resolve(new Headers({ 'x-forwarded-for': '203.0.113.9' }))).toBeUndefined();
  });

  /*
   * Counted from the end because a proxy appends: everything the browser wrote itself lands to
   * the left, so with one hop declared the last entry is the only one this app put there.
   */
  it('takes the entry the declared hop appended', async () => {
    const resolve = await forwardedAddressWithHops(1);
    expect(resolve(new Headers({ 'x-forwarded-for': '198.51.100.7, 203.0.113.9' }))).toBe(
      '203.0.113.9',
    );
  });

  it('counts further back as more hops are declared', async () => {
    const resolve = await forwardedAddressWithHops(2);
    expect(resolve(new Headers({ 'x-forwarded-for': '198.51.100.7, 203.0.113.9' }))).toBe(
      '198.51.100.7',
    );
  });

  // A spoofed header shorter than the declared chain names an address the caller chose, so
  // reporting nothing is what keeps it from burning a victim's allowance.
  it('reports nothing when the chain is shorter than the declared hops', async () => {
    const resolve = await forwardedAddressWithHops(3);
    expect(resolve(new Headers({ 'x-forwarded-for': '198.51.100.7' }))).toBeUndefined();
  });

  it('reports nothing when the header is absent', async () => {
    const resolve = await forwardedAddressWithHops(1);
    expect(resolve(new Headers())).toBeUndefined();
  });

  it('ignores padding and empty entries', async () => {
    const resolve = await forwardedAddressWithHops(1);
    expect(resolve(new Headers({ 'x-forwarded-for': ' 198.51.100.7 ,  , 203.0.113.9 ' }))).toBe(
      '203.0.113.9',
    );
  });
});

describe('sessionCookieOptions', () => {
  it('gives a remembered session an explicit lifetime', () => {
    const options = sessionCookieOptions(true);
    // Narrowed rather than asserted: the return is a union, and only one arm carries maxAge.
    if (!('maxAge' in options)) throw new Error('a remembered session must carry a maxAge');
    expect(options.maxAge).toBeGreaterThan(0);
  });

  // No maxAge is what makes the cookie a session cookie, so it dies with the browser.
  it('leaves a plain session without one', () => {
    expect(sessionCookieOptions(false)).not.toHaveProperty('maxAge');
  });

  it.each([true, false])('keeps the token out of client reach when remembered=%p', (remembered) => {
    expect(sessionCookieOptions(remembered)).toMatchObject({
      httpOnly: true,
      sameSite: 'lax',
      path: '/',
    });
  });
});
