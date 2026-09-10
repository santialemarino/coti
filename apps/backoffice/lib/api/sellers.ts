/*
 * The sellers the manual-order creator can assign a new RFQ to. The data comes through the
 * /api/users BFF proxy, which forwards the caller's auth and branch as the headers the Go
 * API expects; a client component cannot read the HttpOnly cookies itself, so this module is
 * what stands in for a direct call. Branch filtering is enforced upstream by the API.
 */

export interface Seller {
  id: string;
  name: string;
}

// Raw shape returned by GET /v1/sellers via the BFF proxy.
interface SellerListRaw {
  items: Array<{ id: string; name: string }>;
}

export async function listSellers(branchId: string | null): Promise<Seller[]> {
  const params = new URLSearchParams();
  if (branchId) params.set('branch_id', branchId);
  const qs = params.toString();

  const response = await fetch(`/api/users${qs ? `?${qs}` : ''}`, {
    cache: 'no-store',
  });

  if (!response.ok) return [];

  const data = (await response.json()) as SellerListRaw;
  return data.items.map((item) => ({ id: item.id, name: item.name }));
}
