import 'server-only';

import { apiRequest } from '@/lib/api/client';
import { API_URL } from '@/lib/config';

interface ProductRaw {
  id: string;
  code: string | null;
  canonical_name: string;
  description: string | null;
  unit: string | null;
  family_id: string | null;
  subgroup_id: string | null;
  image_path: string | null;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

interface ProductPageRaw {
  items: ProductRaw[];
  total: number;
  limit: number;
  offset: number;
}

interface ProductSubgroupRaw {
  id: string;
  name: string;
}

interface ProductFamilyRaw {
  id: string;
  name: string;
  subgroups: ProductSubgroupRaw[] | null;
}

interface ProductTaxonomyRaw {
  families: ProductFamilyRaw[];
}

export interface Product {
  id: string;
  code: string | null;
  name: string;
  description: string | null;
  unit: string | null;
  familyId: string | null;
  subgroupId: string | null;
  imageUrl: string | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface ProductPage {
  items: Product[];
  total: number;
  limit: number;
  offset: number;
}

export interface ProductSubgroup {
  id: string;
  name: string;
}

export interface ProductFamily {
  id: string;
  name: string;
  subgroups: ProductSubgroup[];
}

function mapProduct(raw: ProductRaw): Product {
  return {
    id: raw.id,
    code: raw.code,
    name: raw.canonical_name,
    description: raw.description,
    unit: raw.unit,
    familyId: raw.family_id,
    subgroupId: raw.subgroup_id,
    imageUrl: raw.image_path ? new URL(raw.image_path, API_URL).toString() : null,
    isActive: raw.is_active,
    createdAt: raw.created_at,
    updatedAt: raw.updated_at,
  };
}

export async function getProducts({
  search = '',
  limit = 50,
  offset = 0,
}: {
  search?: string;
  limit?: number;
  offset?: number;
} = {}): Promise<ProductPage> {
  const raw = await apiRequest<ProductPageRaw>({
    path: '/v1/products',
    branchScoped: false,
    query: {
      search: search || undefined,
      include_inactive: 'true',
      limit: String(limit),
      offset: String(offset),
    },
  });
  return { ...raw, items: raw.items.map(mapProduct) };
}

export async function getProductTaxonomy(): Promise<ProductFamily[]> {
  const raw = await apiRequest<ProductTaxonomyRaw>({
    path: '/v1/product-taxonomy',
    branchScoped: false,
  });
  return raw.families.map((family) => ({
    id: family.id,
    name: family.name,
    subgroups: family.subgroups ?? [],
  }));
}
