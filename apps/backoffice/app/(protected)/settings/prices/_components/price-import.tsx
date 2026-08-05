'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';

import { Callout, PendingButton } from '@repo/ui/components';
import {
  confirmPriceImport,
  exportPrices,
  previewPriceImport,
  type ProductPriceImportPreview,
} from '@/app/(protected)/settings/prices/actions';
import type { Branch } from '@/lib/api/branches';
import { useFormatters } from '@/lib/i18n/formatters';

interface PriceImportProps {
  branch: Branch;
}

export function PriceImport({ branch }: PriceImportProps) {
  const fmt = useFormatters();
  const t = useTranslations('priceImport');
  const [preview, setPreview] = useState<ProductPriceImportPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [successCount, setSuccessCount] = useState<number | null>(null);
  /*
   * One transition per action, not one shared between them. A shared one only says that
   * something is running, so exporting made the preview button announce it was processing —
   * and a flag naming the action cannot fix it, because the form action already runs inside a
   * transition and a state update made there does not commit until that transition ends.
   */
  const [previewing, startPreview] = useTransition();
  const [confirming, startConfirm] = useTransition();
  const [exporting, startExport] = useTransition();
  // The three are mutually exclusive and each invalidates the others' result, so one running
  // locks all three.
  const busy = previewing || confirming || exporting;

  function onPreview(formData: FormData) {
    setError(null);
    setSuccessCount(null);
    startPreview(async () => {
      const result = await previewPriceImport(branch.id, formData);
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
    startConfirm(async () => {
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
    setError(null);
    startExport(async () => {
      const result = await exportPrices(branch.id);
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
      <Callout tone="info">{t('targetBranch', { name: branch.name })}</Callout>

      <form
        action={onPreview}
        noValidate
        className="grid p-5 gap-4 bg-card border rounded-lg md:grid-cols-[1fr_auto] md:items-end"
      >
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
            className="h-10 px-3 py-2 bg-background border border-input rounded-lg outline-none transition-[border-color,box-shadow] duration-200 ease-out-soft focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 file:mr-3 file:border-0 file:bg-transparent file:text-paragraph-sm-medium"
          />
        </div>
        <div className="flex gap-x-2">
          <PendingButton
            type="button"
            variant="outline"
            onClick={onExport}
            disabled={busy}
            pending={exporting}
            pendingLabel={t('form.exporting')}
          >
            {t('form.export')}
          </PendingButton>
          <PendingButton
            type="submit"
            disabled={busy}
            pending={previewing}
            pendingLabel={t('form.previewing')}
          >
            {t('form.preview')}
          </PendingButton>
        </div>
      </form>

      <p className="text-paragraph-sm text-foreground-muted">{t('formatHint')}</p>
      {error ? <Callout tone="danger">{error}</Callout> : null}
      {successCount !== null ? (
        <Callout tone="success">{t('success', { count: successCount })}</Callout>
      ) : null}

      {preview ? (
        <section className="flex flex-col gap-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-paragraph-sm">
              {t('summary', { valid: preview.validRows, invalid: preview.invalidRows })}
            </p>
            <PendingButton
              type="button"
              onClick={onConfirm}
              disabled={busy || !preview.canConfirm}
              pending={confirming}
              pendingLabel={t('confirming')}
            >
              {t('confirm')}
            </PendingButton>
          </div>
          {preview.invalidRows > 0 && preview.canConfirm ? (
            <Callout tone="warning">
              {t('invalidRowsSkipped', { count: preview.invalidRows })}
            </Callout>
          ) : null}
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
                    <td className="px-3 py-2">
                      {row.errors.length === 0 ? (
                        <span>{t('valid')}</span>
                      ) : (
                        <ul className="flex flex-col gap-y-1 text-danger-foreground">
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
