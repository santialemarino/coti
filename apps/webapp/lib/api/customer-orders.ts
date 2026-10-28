import 'server-only';

import { customerApiPath, CustomerAuthError, getCustomerToken } from '@/lib/auth/customer-session';
import { API_INTERNAL_URL } from '@/lib/config/server';

interface CustomerOrderRaw {
  id: string;
  branch_name: string;
  received_at: string;
  status: string;
  original_text: string;
  delivery_token: string | null;
}
export interface CustomerOrder {
  id: string;
  branchName: string;
  receivedAt: string;
  status: string;
  originalText: string;
  deliveryToken: string | null;
}

function mapOrder(raw: CustomerOrderRaw): CustomerOrder {
  return {
    id: raw.id,
    branchName: raw.branch_name,
    receivedAt: raw.received_at,
    status: raw.status,
    originalText: raw.original_text,
    deliveryToken: raw.delivery_token,
  };
}

export async function customerFetch(path: string, options?: RequestInit): Promise<Response> {
  const token = await getCustomerToken();
  if (!token) throw new CustomerAuthError(401);
  return fetch(await customerApiPath(path), {
    ...options,
    cache: 'no-store',
    headers: { ...options?.headers, Authorization: `Bearer ${token}` },
  });
}

export async function getCustomerOrders(page: number): Promise<CustomerOrder[]> {
  const response = await customerFetch(`/orders?page=${page}`);
  if (!response.ok) throw new CustomerAuthError(response.status);
  return ((await response.json()) as CustomerOrderRaw[]).map(mapOrder);
}

export async function getCustomerOrder(id: string): Promise<CustomerOrder | null> {
  const response = await customerFetch(`/orders/${encodeURIComponent(id)}`);
  if (response.status === 404) return null;
  if (!response.ok) throw new CustomerAuthError(response.status);
  return mapOrder((await response.json()) as CustomerOrderRaw);
}

export async function getCustomerOrderByToken(token: string): Promise<CustomerOrder | null> {
  const response = await customerFetch(`/quote-sends/${encodeURIComponent(token)}/order`);
  if (response.status === 404) return null;
  if (!response.ok) throw new CustomerAuthError(response.status);
  return mapOrder((await response.json()) as CustomerOrderRaw);
}

export async function getDeliveryHostname(token: string): Promise<string | null> {
  const response = await fetch(
    API_INTERNAL_URL + '/v1/public/quote-sends/' + encodeURIComponent(token) + '/site',
    { cache: 'no-store' },
  );
  if (response.status === 404) return null;
  if (!response.ok) throw new Error('Delivery supplier routing unavailable');
  const raw = (await response.json()) as { hostname: string };
  return raw.hostname;
}
