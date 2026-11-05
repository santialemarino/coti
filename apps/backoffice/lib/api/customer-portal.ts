import 'server-only';

import { apiRequest } from '@/lib/api/client';

interface CustomerPortalRaw {
  base_url: string;
  hostname: string;
  url: string;
  subdomain: string;
  legacy: boolean;
}

export interface CustomerPortal {
  baseUrl: string;
  hostname: string;
  url: string;
  subdomain: string;
  legacy: boolean;
}

export function mapCustomerPortal(raw: CustomerPortalRaw): CustomerPortal {
  return {
    baseUrl: raw.base_url,
    hostname: raw.hostname,
    url: raw.url,
    subdomain: raw.subdomain,
    legacy: raw.legacy,
  };
}

export async function getCustomerPortal(): Promise<CustomerPortal> {
  return mapCustomerPortal(
    await apiRequest<CustomerPortalRaw>({ path: '/v1/account/portal', branchScoped: false }),
  );
}
