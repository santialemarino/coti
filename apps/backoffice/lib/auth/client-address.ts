import 'server-only';

import { headers } from 'next/headers';

import { forwardedClientAddress } from '@/lib/auth/tokens';

// The browser's address for a server-side call, so the API rate-limits per user rather than
// per Next server. See forwardedClientAddress for how the hop is chosen.
export async function clientAddress(): Promise<string | undefined> {
  return forwardedClientAddress(await headers());
}
