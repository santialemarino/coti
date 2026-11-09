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
  familyId?: string | null;
  subgroupId?: string | null;
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
  family_id: string | null;
  subgroup_id: string | null;
}

interface ProductListRaw {
  items: ProductRaw[];
  total: number;
  limit: number;
  offset: number;
}

export interface CatalogCompatibility {
  familyId: string;
  subgroupId: string;
  unit: string;
}

function mapProduct(item: ProductRaw): CatalogProduct {
  return {
    id: item.id,
    code: item.code ?? '',
    name: item.canonical_name,
    unit: item.unit ?? '',
    familyId: item.family_id,
    subgroupId: item.subgroup_id,
  };
}

export async function getCatalogProduct(id: string): Promise<CatalogProduct | null> {
  try {
    const response = await fetch(`/api/products/${encodeURIComponent(id)}`, { cache: 'no-store' });
    if (!response.ok) return null;
    return mapProduct((await response.json()) as ProductRaw);
  } catch {
    return null;
  }
}

export function catalogCompatibility(product: CatalogProduct | null): CatalogCompatibility | null {
  if (!product?.familyId || !product.subgroupId || !product.unit.trim()) return null;
  return { familyId: product.familyId, subgroupId: product.subgroupId, unit: product.unit };
}

export async function searchCatalog(
  query: string,
  compatibility?: CatalogCompatibility,
): Promise<CatalogProduct[]> {
  const q = query.trim();
  const params = new URLSearchParams();
  if (q) params.set('search', q);
  params.set('limit', '50');
  if (compatibility) {
    params.set('family_id', compatibility.familyId);
    params.set('subgroup_id', compatibility.subgroupId);
  }

  const results: CatalogProduct[] = [];
  let offset = 0;
  do {
    if (offset) params.set('offset', String(offset));
    const response = await fetch(`/api/products?${params.toString()}`, { cache: 'no-store' });
    if (!response.ok) throw new Error('Catalog search failed');
    const data = (await response.json()) as ProductListRaw;
    results.push(
      ...data.items
        .map(mapProduct)
        .filter(
          (item) =>
            !compatibility ||
            item.unit.trim().toLowerCase() === compatibility.unit.trim().toLowerCase(),
        ),
    );
    offset += data.items.length;
    if (!compatibility || data.items.length === 0 || offset >= data.total) break;
  } while (results.length < 50);
  return results.slice(0, 50);
}
