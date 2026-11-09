import 'server-only';

import { cookies } from 'next/headers';

import { getPortalSlug } from '@/lib/portal/server';

export async function customerCookies() {
  const jar = await cookies();
  const slug = await getPortalSlug();
  const name = (value: string) => (slug ? `${value}_${slug}` : value);
  return {
    get: (key: string) => jar.get(name(key)),
    set: (key: string, value: string, options: Parameters<typeof jar.set>[2]) =>
      jar.set(name(key), value, options),
  };
}
