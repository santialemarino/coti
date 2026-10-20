import { isIP } from 'node:net';

import { TRUSTED_PROXY_HOPS } from '@/lib/config';

export function forwardedClientAddress(
  headers: Headers,
  trustedProxyHops = TRUSTED_PROXY_HOPS,
): string | undefined {
  if (trustedProxyHops <= 0) return undefined;
  const hops =
    headers
      .get('x-forwarded-for')
      ?.split(',')
      .map((entry) => entry.trim())
      .filter(Boolean) ?? [];
  const address = hops[hops.length - trustedProxyHops];
  return address && isIP(address) ? address : undefined;
}
