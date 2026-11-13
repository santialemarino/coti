/*
 * The navigation in flight as plain data, moved along by `NavigationPendingProvider`. A navigation is
 * over once the COMMITTED URL moves off the one it left — the destination, a redirect or the error
 * boundary alike; one abandoned in place is settled explicitly.
 */

export interface PendingNavigation {
  id: number;
  // The committed path + query it left.
  from: string;
  // The path + query it is going to.
  to: string;
  // Another page, not the same one with another query.
  page: boolean;
}

export interface ClickLike {
  button: number;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  altKey: boolean;
}

export interface AnchorLike {
  href: string;
  target: string;
  hasAttribute(name: string): boolean;
}

// Re-serialised the way `useSearchParams().toString()` writes it, so `?q=a,b` equals `?q=a%2Cb`.
export function pathAndQuery(url: URL): string {
  const query = new URLSearchParams(url.search).toString();
  return query ? `${url.pathname}?${query}` : url.pathname;
}

export function pathOf(pathAndQueryValue: string): string {
  const query = pathAndQueryValue.indexOf('?');
  return query === -1 ? pathAndQueryValue : pathAndQueryValue.slice(0, query);
}

/*
 * Where a click on `anchor` takes this tab, or null when it does not: another button, a modifier, a
 * target, a download, another origin, or an in-page `#` jump.
 */
export function linkDestination(click: ClickLike, anchor: AnchorLike, current: URL): string | null {
  if (click.button !== 0 || click.metaKey || click.ctrlKey || click.shiftKey || click.altKey) {
    return null;
  }
  if ((anchor.target && anchor.target !== '_self') || anchor.hasAttribute('download')) return null;
  let url: URL;
  try {
    url = new URL(anchor.href, current);
  } catch {
    return null;
  }
  if (url.origin !== current.origin) return null;
  if (url.hash && pathAndQuery(url) === pathAndQuery(current)) return null;
  return pathAndQuery(url);
}

// One to the URL already committed is nothing to wait for, but as the newest it ends any other.
export function beginNavigation(id: number, from: string, to: string): PendingNavigation | null {
  if (to === from) return null;
  return { id, from, to, page: pathOf(to) !== pathOf(from) };
}

// Ends `id` unless a newer navigation has replaced it.
export function settleNavigation(
  pending: PendingNavigation | null,
  id: number,
): PendingNavigation | null {
  return pending?.id === id ? null : pending;
}

export function activeNavigation(
  pending: PendingNavigation | null,
  current: string,
): PendingNavigation | null {
  return pending && pending.from === current ? pending : null;
}
