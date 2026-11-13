import { describe, expect, it } from 'vitest';

import {
  activeNavigation,
  beginNavigation,
  linkDestination,
  pathOf,
  settleNavigation,
  type AnchorLike,
  type ClickLike,
} from '@/lib/navigation-pending';

const HERE = new URL('http://localhost:3000/clients?page=2');
const PLAIN: ClickLike = {
  button: 0,
  metaKey: false,
  ctrlKey: false,
  shiftKey: false,
  altKey: false,
};

function anchor(href: string, attributes: Record<string, string> = {}): AnchorLike {
  return {
    href: new URL(href, HERE).href,
    target: attributes.target ?? '',
    hasAttribute: (name) => name in attributes,
  };
}

describe('linkDestination', () => {
  it('is the path + query of an in-app link', () => {
    expect(linkDestination(PLAIN, anchor('/reports'), HERE)).toBe('/reports');
    expect(linkDestination(PLAIN, anchor('/rfqs?status=SENT'), HERE)).toBe('/rfqs?status=SENT');
    expect(linkDestination(PLAIN, anchor('/reports#top'), HERE)).toBe('/reports');
  });

  it('ignores a click that opens elsewhere: modifiers, another button, a target, a download', () => {
    for (const key of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey'] as const) {
      expect(linkDestination({ ...PLAIN, [key]: true }, anchor('/reports'), HERE), key).toBeNull();
    }
    expect(linkDestination({ ...PLAIN, button: 1 }, anchor('/reports'), HERE)).toBeNull();
    expect(linkDestination(PLAIN, anchor('/reports', { target: '_blank' }), HERE)).toBeNull();
    expect(linkDestination(PLAIN, anchor('/export.csv', { download: '' }), HERE)).toBeNull();
    expect(linkDestination(PLAIN, anchor('/reports', { target: '_self' }), HERE)).toBe('/reports');
  });

  it('ignores another origin and an in-page jump', () => {
    expect(linkDestination(PLAIN, anchor('https://example.com/reports'), HERE)).toBeNull();
    expect(linkDestination(PLAIN, anchor('#main'), HERE)).toBeNull();
    expect(linkDestination(PLAIN, anchor('/clients?page=2#row-3'), HERE)).toBeNull();
  });

  // Otherwise a click on the page on screen starts a navigation that can never end.
  it('writes the query as the router does, so a link to the committed URL compares equal', () => {
    const committed = (search: string) => `/reports?${new URLSearchParams(search).toString()}`;
    for (const href of ['/reports?status=SENT,ACCEPTED', '/reports?client=Obra%20Norte']) {
      const here = new URL(href, HERE);
      const to = linkDestination(PLAIN, anchor(href), here);
      expect(to, href).toBe(committed(here.search));
      expect(beginNavigation(1, committed(here.search), to!), href).toBeNull();
    }
  });
});

describe('a pending navigation', () => {
  it('starts, knowing whether it changes page', () => {
    expect(beginNavigation(1, '/clients', '/reports')).toEqual({
      id: 1,
      from: '/clients',
      to: '/reports',
      page: true,
    });
    expect(beginNavigation(2, '/reports', '/reports?granularity=week')?.page).toBe(false);
    expect(pathOf('/reports?granularity=week')).toBe('/reports');
  });

  it('is nothing to wait for when it goes to the URL already committed', () => {
    expect(beginNavigation(3, '/reports', '/reports')).toBeNull();
  });

  it('runs while the committed URL is the one it left, and is over once that moves', () => {
    const pending = beginNavigation(1, '/clients', '/reports');
    expect(activeNavigation(pending, '/clients')).toBe(pending);
    expect(activeNavigation(pending, '/reports')).toBeNull();
    // A redirect ends it all the same.
    expect(activeNavigation(pending, '/session-ended')).toBeNull();
    expect(activeNavigation(null, '/clients')).toBeNull();
  });

  it('is replaced by a newer one, whose settle alone can end it', () => {
    const first = beginNavigation(1, '/clients', '/reports');
    const second = beginNavigation(2, '/clients', '/rfqs');
    expect(settleNavigation(second, 1)).toBe(second);
    expect(settleNavigation(second, 2)).toBeNull();
    expect(settleNavigation(first, 1)).toBeNull();
    expect(settleNavigation(null, 1)).toBeNull();
  });
});
