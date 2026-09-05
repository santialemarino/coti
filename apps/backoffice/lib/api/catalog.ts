/*
 * The catalog interface consumed by the manual-order builder. The search goes
 * through a BFF route that proxies GET /v1/products, so swapping it for a
 * direct call is a change of this module alone — the components only know
 * this interface.
 *
 * Price is per-branch (GET /v1/products/:productId/prices) and is not part of
 * the product listing; it is optional here.
 */

export interface CatalogProduct {
  id: string;
  code: string;
  name: string;
  /* Free text (bolsa, m2, kg, barra, ...), mirroring the backend's nullable `unit`. */
  unit: string;
  /* Optional: not available from the product listing endpoint. */
  category?: string;
  /* Decimal string, the wire format money travels as (NUMERIC(14,2)); per-branch. */
  price?: string;
}

// Raw shape returned by GET /v1/products via the BFF proxy.
interface ProductRaw {
  id: string;
  code: string | null;
  canonical_name: string;
  unit: string | null;
}

interface ProductListRaw {
  items: ProductRaw[];
  total: number;
  limit: number;
  offset: number;
}

export async function searchCatalog(query: string): Promise<CatalogProduct[]> {
  const q = query.trim();
  const params = new URLSearchParams();
  if (q) params.set('search', q);
  params.set('limit', '50');

  const response = await fetch(`/api/products?${params.toString()}`, {
    cache: 'no-store',
  });

  if (!response.ok) return [];

  const data = (await response.json()) as ProductListRaw;
  return data.items.map((item) => ({
    id: item.id,
    code: item.code ?? '',
    name: item.canonical_name,
    unit: item.unit ?? '',
  }));
}
