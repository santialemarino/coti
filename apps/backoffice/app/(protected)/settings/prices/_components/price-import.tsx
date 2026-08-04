'use client';

import { useRef, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import {
  confirmPriceImport,
  exportPrices,
  previewPriceImport,
  type ProductPriceImportPreview,
} from '@/app/(protected)/settings/prices/actions';
import type { Branch } from '@/lib/api/branches';
import { useFormatters } from '@/lib/i18n/formatters';

type PriceImportProps = {
  branches: Branch[];
};

export function PriceImport({ branches }: PriceImportProps) {
  const fmt = useFormatters();
  const t = useTranslations('priceImport');
  const [preview, setPreview] = useState<ProductPriceImportPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [successCount, setSuccessCount] = useState<number | null>(null);
  const formRef = useRef<HTMLFormElement>(null);
  const [pending, startTransition] = useTransition();

  function onPreview(formData: FormData) {
    setError(null);
    setSuccessCount(null);
    startTransition(async () => {
      const result = await previewPriceImport(formData);
      if (!result.ok) {
        setPreview(null);
        setError(t(`errors.${result.error}`));
        return;
      }
      setPreview(result.preview);
    });
  }

  function onConfirm() {
    if (!preview) return;
    setError(null);
    startTransition(async () => {
      const result = await confirmPriceImport(preview);
      if (!result.ok) {
        setError(t(`errors.${result.error}`));
        return;
      }
      setSuccessCount(result.importedRows);
      setPreview(null);
    });
  }

  function onExport() {
    if (!formRef.current) return;
    const branchId = String(new FormData(formRef.current).get('branchId') ?? '');
    if (!branchId) {
      setError(t('errors.selectBranch'));
      return;
    }
    setError(null);
    startTransition(async () => {
      const result = await exportPrices(branchId);
      if (!result.ok) {
        setError(t(`errors.${result.error}`));
        return;
      }
      const binary = atob(result.contentBase64);
      const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
      const url = URL.createObjectURL(
        new Blob([bytes], {
          type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        }),
      );
      const link = document.createElement('a');
      link.href = url;
      link.download = result.filename;
      link.click();
      URL.revokeObjectURL(url);
    });
  }

  return (
    <div className="flex flex-col gap-y-6">
      <form
        ref={formRef}
        action={onPreview}
        noValidate
        className="grid p-5 gap-4 bg-card border rounded-lg md:grid-cols-[1fr_1fr_auto] md:items-end"
      >
        <div className="flex flex-col gap-y-1">
          <label htmlFor="branchId" className="text-paragraph-sm-medium">
            {t('form.branch.label')}
          </label>
          <select
            id="branchId"
            name="branchId"
            required
            defaultValue=""
            className="h-10 px-3 bg-background border border-input rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="" disabled>
              {t('form.branch.placeholder')}
            </option>
            {branches.map((branch) => (
              <option key={branch.id} value={branch.id}>
                {branch.name}
              </option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-y-1">
          <label htmlFor="file" className="text-paragraph-sm-medium">
            {t('form.file.label')}
          </label>
          <input
            id="file"
            name="file"
            type="file"
            required
            accept=".xlsx,.csv"
            className="h-10 px-3 py-2 bg-background border border-input rounded-md file:mr-3 file:border-0 file:bg-transparent file:text-paragraph-sm-medium"
          />
        </div>
        <div className="flex gap-x-2">
          <Button type="button" variant="outline" onClick={onExport} disabled={pending}>
            {t('form.export')}
          </Button>
          <Button type="submit" disabled={pending}>
            {pending ? t('form.previewing') : t('form.preview')}
          </Button>
        </div>
      </form>

      <p className="text-paragraph-sm text-foreground-muted">{t('formatHint')}</p>
      {error ? (
        <p className="p-3 bg-destructive/10 border border-destructive/30 rounded-md text-paragraph-sm text-danger-foreground">
          {error}
        </p>
      ) : null}
      {successCount !== null ? (
        <p className="p-3 bg-primary/10 border border-primary/30 rounded-md text-paragraph-sm">
          {t('success', { count: successCount })}
        </p>
      ) : null}

      {preview ? (
        <section className="flex flex-col gap-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-paragraph-sm">
              {t('summary', { valid: preview.validRows, invalid: preview.invalidRows })}
            </p>
            <Button type="button" onClick={onConfirm} disabled={pending || !preview.canConfirm}>
              {pending ? t('confirming') : t('confirm')}
            </Button>
          </div>
          <div className="overflow-x-auto border rounded-lg">
            <table className="w-full border-collapse text-paragraph-sm">
              <thead className="bg-muted">
                <tr>
                  <th className="px-3 py-2 text-left">{t('table.row')}</th>
                  <th className="px-3 py-2 text-left">{t('table.code')}</th>
                  <th className="px-3 py-2 text-left">{t('table.product')}</th>
                  <th className="px-3 py-2 text-left">{t('table.currentPrice')}</th>
                  <th className="px-3 py-2 text-left">{t('table.newPrice')}</th>
                  <th className="px-3 py-2 text-left">{t('table.minPrice')}</th>
                  <th className="px-3 py-2 text-left">{t('table.conditions')}</th>
                  <th className="px-3 py-2 text-left">{t('table.result')}</th>
                </tr>
              </thead>
              <tbody>
                {preview.rows.map((row) => (
                  <tr key={`${row.rowNumber}-${row.code}`} className="border-t">
                    <td className="px-3 py-2">{row.rowNumber}</td>
                    <td className="px-3 py-2 text-paragraph-sm-medium">{row.code || '—'}</td>
                    <td className="px-3 py-2">{row.productName || '—'}</td>
                    <td className="px-3 py-2">
                      {row.currentPrice ? fmt.currency(row.currentPrice, row.currency) : '—'}
                    </td>
                    <td className="px-3 py-2">
                      {row.price ? fmt.currency(row.price, row.currency) : '—'}
                    </td>
                    <td className="px-3 py-2">
                      {row.minPrice ? fmt.currency(row.minPrice, row.currency) : '—'}
                    </td>
                    <td className="px-3 py-2">{row.conditions ?? '—'}</td>
                    <td className="px-3 py-2">
                      {row.errors.length === 0 ? (
                        <span>{t('valid')}</span>
                      ) : (
                        <ul className="flex flex-col gap-y-1 text-destructive">
                          {row.errors.map((message) => (
                            <li key={message}>{t(`rowErrors.${message}`)}</li>
                          ))}
                        </ul>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {!preview.canConfirm ? (
            <p className="text-paragraph-sm text-danger-foreground">{t('fixErrors')}</p>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
