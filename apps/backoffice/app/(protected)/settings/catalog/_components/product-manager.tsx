'use client';

import { useCallback, useEffect, useMemo, useState, useTransition } from 'react';
import Image from 'next/image';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import {
  ImageIcon,
  PackageOpenIcon,
  PencilIcon,
  PlusIcon,
  RotateCcwIcon,
  SearchXIcon,
  Trash2Icon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Callout,
  Card,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  Pagination,
  RowActionButton,
  SearchInput,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { ProductFormDialog } from '@/app/(protected)/settings/catalog/_components/product-form-dialog';
import {
  createProduct,
  deactivateProduct,
  reactivateProduct,
  updateProduct,
  uploadProductImage,
} from '@/app/(protected)/settings/catalog/actions';
import type { ProductValues } from '@/app/(protected)/settings/catalog/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Product, ProductFamily, ProductPage } from '@/lib/api/products';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

const PAGE_SIZE = 20;

interface ProductManagerProps {
  page: ProductPage;
  families: ProductFamily[];
  query: string;
}

export function ProductManager({ page, families, query }: ProductManagerProps) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const t = useTranslations('products');
  const tCommon = useTranslations('common');
  const message = useApiErrorMessage('products');
  const [search, setSearch] = useState(query);
  const [form, setForm] = useState<{ mode: 'create' | 'edit'; product: Product | null } | null>(
    null,
  );
  const [deactivating, setDeactivating] = useState<Product | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, startSave] = useTransition();
  const [removing, startRemove] = useTransition();
  const [reactivating, startReactivate] = useTransition();
  const busy = saving || removing || reactivating;
  const currentPage = Math.floor(page.offset / PAGE_SIZE) + 1;
  const pageCount = Math.max(1, Math.ceil(page.total / PAGE_SIZE));
  const familyNames = useMemo(
    () => new Map(families.map((family) => [family.id, family.name])),
    [families],
  );

  const navigate = useCallback(
    (nextPage: number, nextQuery = query) => {
      const params = new URLSearchParams(searchParams.toString());
      if (nextQuery) params.set('q', nextQuery);
      else params.delete('q');
      if (nextPage > 1) params.set('page', String(nextPage));
      else params.delete('page');
      const encoded = params.toString();
      router.replace(encoded ? `${pathname}?${encoded}` : pathname);
    },
    [pathname, query, router, searchParams],
  );

  useEffect(() => setSearch(query), [query]);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      if (search.trim() === query) return;
      navigate(1, search.trim());
    }, 300);
    return () => window.clearTimeout(timeout);
  }, [search, query, navigate]);

  function onSubmit(values: ProductValues, image: File | null) {
    const target = form;
    if (!target) return;
    setError(null);
    startSave(async () => {
      const result = target.product
        ? await updateProduct(target.product.id, values)
        : await createProduct(values);
      if (!result.ok || !result.productId) {
        setError(message(result.error));
        return;
      }
      if (image) {
        const payload = new FormData();
        payload.set('file', image);
        const upload = await uploadProductImage(result.productId, payload);
        if (!upload.ok) {
          setError(message(upload.error));
          if (!target.product) {
            setForm(null);
            router.refresh();
          }
          return;
        }
      }
      toast.success(t(target.mode === 'create' ? 'created' : 'updated', { name: values.name }));
      setForm(null);
      router.refresh();
    });
  }

  function onDeactivate() {
    const target = deactivating;
    if (!target) return;
    setError(null);
    startRemove(async () => {
      const result = await deactivateProduct(target.id);
      if (!result.ok) {
        setError(message(result.error));
        setDeactivating(null);
        return;
      }
      toast.success(t('deactivated', { name: target.name }));
      setDeactivating(null);
      router.refresh();
    });
  }

  function onReactivate(product: Product) {
    setError(null);
    startReactivate(async () => {
      const result = await reactivateProduct(product.id, valuesOf(product));
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      toast.success(t('reactivated', { name: product.name }));
      router.refresh();
    });
  }

  return (
    <Card className="gap-y-0 overflow-hidden py-0">
      <CardHeader className="flex-row items-center justify-between py-6">
        <div className="flex items-center gap-x-3">
          <CardTitle className="text-heading-3">{t('title')}</CardTitle>
          <Badge tone="neutral">{t('total', { total: page.total })}</Badge>
        </div>
        <Button disabled={busy} onClick={() => setForm({ mode: 'create', product: null })}>
          <PlusIcon aria-hidden="true" />
          {t('add')}
        </Button>
      </CardHeader>

      <div className="border-y border-border px-6 py-5">
        <SearchInput
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          onClear={() => setSearch('')}
          clearLabel={tCommon('form.clearSearch')}
          placeholder={t('search')}
          maxLength={TEXT_FIELD_MAX_LENGTH}
          containerClassName="w-full"
        />
      </div>

      {error ? (
        <Callout tone="danger" className="m-6 mb-0">
          {error}
        </Callout>
      ) : null}

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('table.product')}</TableHead>
            <TableHead>{t('table.code')}</TableHead>
            <TableHead>{t('table.family')}</TableHead>
            <TableHead>{t('table.unit')}</TableHead>
            <TableHead>{t('table.status')}</TableHead>
            <TableHead className="text-right">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.items.length === 0 ? (
            <TableEmptyRow
              colSpan={6}
              icon={query ? SearchXIcon : PackageOpenIcon}
              title={t(query ? 'noResults.title' : 'empty.title')}
              description={t(query ? 'noResults.description' : 'empty.description')}
            />
          ) : (
            page.items.map((product) => (
              <TableRow key={product.id}>
                <TableCell>
                  <div className="flex items-center gap-x-3">
                    <span className="grid size-10 shrink-0 place-items-center overflow-hidden bg-muted rounded-lg text-foreground-subtle">
                      {product.imageUrl ? (
                        <Image
                          src={product.imageUrl}
                          alt=""
                          width={40}
                          height={40}
                          unoptimized
                          className="size-10 object-cover"
                        />
                      ) : (
                        <ImageIcon aria-hidden="true" className="size-4" />
                      )}
                    </span>
                    <span className="text-paragraph-sm-medium text-foreground">{product.name}</span>
                  </div>
                </TableCell>
                <TableCell className={product.code ? undefined : 'text-foreground-subtle'}>
                  {product.code ?? '—'}
                </TableCell>
                <TableCell className={product.familyId ? undefined : 'text-foreground-subtle'}>
                  {product.familyId ? (familyNames.get(product.familyId) ?? '—') : '—'}
                </TableCell>
                <TableCell className={product.unit ? undefined : 'text-foreground-subtle'}>
                  {product.unit ?? '—'}
                </TableCell>
                <TableCell>
                  <Badge tone={product.isActive ? 'success' : 'neutral'}>
                    {t(product.isActive ? 'status.active' : 'status.inactive')}
                  </Badge>
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-x-1">
                    <RowActionButton
                      icon={PencilIcon}
                      label={t('edit.action')}
                      disabled={busy}
                      onClick={() => setForm({ mode: 'edit', product })}
                    />
                    {product.isActive ? (
                      <RowActionButton
                        icon={Trash2Icon}
                        label={t('deactivate.action')}
                        tone="danger"
                        disabled={busy}
                        onClick={() => setDeactivating(product)}
                      />
                    ) : (
                      <RowActionButton
                        icon={RotateCcwIcon}
                        label={t('reactivate.action')}
                        disabled={busy}
                        onClick={() => onReactivate(product)}
                      />
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      {page.total > PAGE_SIZE ? (
        <div className="border-t border-border px-6 py-4">
          <Pagination
            page={currentPage}
            pageCount={pageCount}
            onPageChange={(next) => navigate(next)}
            labels={{
              previous: tCommon('pagination.previous'),
              next: tCommon('pagination.next'),
              page: tCommon('pagination.label'),
            }}
          />
        </div>
      ) : null}

      <ProductFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && !saving && setForm(null)}
        mode={form?.mode ?? 'create'}
        product={form?.product ?? null}
        families={families}
        pending={saving}
        onSubmit={onSubmit}
      />

      <ConfirmDialog
        open={deactivating !== null}
        onOpenChange={(open) => !open && !removing && setDeactivating(null)}
        entity={deactivating}
        title={t('deactivate.title')}
        description={(product) => t('deactivate.description', { name: product.name })}
        onConfirm={onDeactivate}
        pending={removing}
        labels={{
          confirm: t('deactivate.confirm'),
          pending: t('deactivate.confirming'),
          cancel: t('cancel'),
        }}
      />
    </Card>
  );
}

function valuesOf(product: Product): ProductValues {
  return {
    code: product.code ?? '',
    name: product.name,
    description: product.description ?? '',
    unit: product.unit ?? '',
    familyId: product.familyId ?? '',
    subgroupId: product.subgroupId ?? '',
    isActive: true,
  };
}
