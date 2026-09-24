import { getTranslations } from 'next-intl/server';

import { Callout, Card, CardContent, CardHeader, CardTitle } from '@repo/ui/components';
import { ProductManager } from '@/app/(protected)/settings/catalog/_components/product-manager';
import { CatalogImport } from '@/components/catalog-import';
import { getBranches } from '@/lib/api/branches';
import { getProducts, getProductTaxonomy } from '@/lib/api/products';
import { getEffectiveBranchId } from '@/lib/auth/branch';
import { requireAdmin } from '@/lib/auth/session';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { generatePageMetadata } from '@/lib/utils/page';

export const generateMetadata = () => generatePageMetadata('catalogSettings');

export default async function CatalogSettingsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  await requireAdmin();
  const t = await getTranslations('catalogImport');
  const params = await searchParams;
  const rawQuery = Array.isArray(params.q) ? params.q[0] : params.q;
  const rawPage = Array.isArray(params.page) ? params.page[0] : params.page;
  const query = (rawQuery?.trim() ?? '').slice(0, TEXT_FIELD_MAX_LENGTH);
  const page = Math.max(1, Number.parseInt(rawPage ?? '1', 10) || 1);
  const [branches, products, families] = await Promise.all([
    getBranches(),
    getProducts({ search: query, limit: 20, offset: (page - 1) * 20 }),
    getProductTaxonomy(),
  ]);
  const activeBranchId = await getEffectiveBranchId(branches);
  const branch = branches.find((candidate) => candidate.id === activeBranchId);

  return (
    <main className="flex flex-col gap-y-8">
      <h1 className="text-heading-2">{t('title')}</h1>
      <ProductManager page={products} families={families} query={query} />
      {branch ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-heading-3">{t('bulk.title')}</CardTitle>
          </CardHeader>
          <CardContent>
            <CatalogImport branch={branch} />
          </CardContent>
        </Card>
      ) : (
        <Callout tone="warning" title={t('noBranch.title')}>
          {t('noBranch.description')}
        </Callout>
      )}
    </main>
  );
}
