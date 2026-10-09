import { describe, expect, it } from 'vitest';

import {
  AUTH_ROUTES,
  isOrdersPath,
  isProtectedPath,
  isSettingsPath,
  PROTECTED_ROUTES,
  PUBLIC_ROUTES,
  queueSelection,
  ROUTES,
  safeNextPath,
  SESSION_CLEARING_ROUTES,
  SIGNED_OUT_ONLY_ROUTES,
} from '@/config/routes';

describe('reachability', () => {
  // Someone with no account is the only caller registration has, so the gate cannot ask for a
  // session — and someone who already has one has no business filling the wizard in again.
  it('lets signup through without a session and bounces a caller who has one', () => {
    expect(AUTH_ROUTES).toContain(ROUTES.signup);
    expect(SIGNED_OUT_ONLY_ROUTES).toContain(ROUTES.signup);
  });

  /*
   * verify-email is the deliberate exception: signup opens a session and sends the caller
   * there, so bouncing a signed-in one would make the screen unreachable exactly when it is
   * needed. Pinned because it looks like an omission.
   */
  it('leaves verify-email public but reachable with a session', () => {
    expect(AUTH_ROUTES).toContain(ROUTES.verifyEmail);
    expect(SIGNED_OUT_ONLY_ROUTES).not.toContain(ROUTES.verifyEmail);
  });

  // A recovery or invite link may open in a browser that is signed in — often the admin's own,
  // trying the invite — and bouncing home would swallow the link.
  it('lets a mailed reset or invite link open with a session', () => {
    expect(AUTH_ROUTES).toContain(ROUTES.resetPassword);
    expect(SIGNED_OUT_ONLY_ROUTES).not.toContain(ROUTES.resetPassword);
  });
});

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

describe('queueSelection', () => {
  it('reads the landing as the queue with nothing open', () => {
    expect(queueSelection(ROUTES.home)).toEqual({ inQueue: true, rfqId: null });
  });

  it('reads an order as the queue with that order open', () => {
    expect(queueSelection(ROUTES.rfqsDetail('r1'))).toEqual({ inQueue: true, rfqId: 'r1' });
  });

  // The table and anything nested under an order are not the queue's screens.
  it.each([ROUTES.rfqs, `${ROUTES.rfqs}/`, '/rfqs/r1/extra', ROUTES.clients])(
    'reads %s as outside the queue',
    (path) => {
      expect(queueSelection(path)).toEqual({ inQueue: false, rfqId: null });
    },
  );
});

describe('isOrdersPath', () => {
  it('covers the queue, the table and an order, and nothing else', () => {
    expect(isOrdersPath(ROUTES.home)).toBe(true);
    expect(isOrdersPath(ROUTES.rfqs)).toBe(true);
    expect(isOrdersPath(ROUTES.rfqsDetail('r1'))).toBe(true);
    expect(isOrdersPath('/rfqsx')).toBe(false);
    expect(isOrdersPath(ROUTES.clients)).toBe(false);
  });
});

describe('isSettingsPath', () => {
  it('covers the settings root and its pages, and nothing that only shares its prefix', () => {
    expect(isSettingsPath(ROUTES.accountSettings)).toBe(true);
    expect(isSettingsPath(ROUTES.settings)).toBe(true);
    expect(isSettingsPath('/settingsx')).toBe(false);
    expect(isSettingsPath(ROUTES.clients)).toBe(false);
  });
});

describe('isProtectedPath', () => {
  it('guards the queue, every registered app route and the route handlers', () => {
    expect(isProtectedPath(ROUTES.home)).toBe(true);
    expect(isProtectedPath(ROUTES.rfqsDetail('a1'))).toBe(true);
    expect(isProtectedPath(ROUTES.userSettings)).toBe(true);
    expect(isProtectedPath(ROUTES.onboarding)).toBe(true);
    expect(isProtectedPath('/api/rfqs')).toBe(true);
  });

  // An unknown path is answered with the 404 rather than a login screen for a page that never was.
  it('leaves the public site and any unknown path unguarded', () => {
    expect(isProtectedPath(ROUTES.landing)).toBe(false);
    expect(isProtectedPath(ROUTES.privacy)).toBe(false);
    expect(isProtectedPath(ROUTES.login)).toBe(false);
    expect(isProtectedPath('/precios')).toBe(false);
    expect(isProtectedPath('/rfqsx')).toBe(false);
  });

  // The public pages have to open for a seller too: the 404 and the header send them there.
  it('keeps the public site reachable with a session', () => {
    [ROUTES.landing, ROUTES.privacy, ROUTES.terms].forEach((route) => {
      expect(PUBLIC_ROUTES).toContain(route);
      expect(SIGNED_OUT_ONLY_ROUTES).not.toContain(route);
    });
  });
});

describe('route groups', () => {
  const groups = [AUTH_ROUTES, PUBLIC_ROUTES, PROTECTED_ROUTES];

  // A route in no group, or in two, is a route nobody decided the gate's answer for.
  it('puts every registered route in exactly one group', () => {
    Object.values(ROUTES)
      .flatMap((route) => (typeof route === 'string' ? [route] : []))
      .forEach((route) => {
        expect(groups.filter((group) => group.includes(route))).toHaveLength(1);
      });
  });

  it('keeps the signed-out-only and session-clearing routes inside the auth group', () => {
    [...SIGNED_OUT_ONLY_ROUTES, ...SESSION_CLEARING_ROUTES].forEach((route) => {
      expect(AUTH_ROUTES).toContain(route);
    });
  });
});

describe('safeNextPath allowlist', () => {
  // `next` only ever points back at a guarded page; anything else is not ours to honour.
  it('keeps a guarded page and refuses a public or auth one', () => {
    expect(safeNextPath('/rfqs/a1?tab=items')).toBe('/rfqs/a1?tab=items');
    expect(safeNextPath(ROUTES.login)).toBe(ROUTES.home);
    expect(safeNextPath(ROUTES.landing)).toBe(ROUTES.home);
    expect(safeNextPath('/precios')).toBe(ROUTES.home);
  });
});
