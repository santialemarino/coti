'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';

import {
  Callout,
  Dropzone,
  PendingButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import {
  confirmPriceImport,
  exportPrices,
  previewPriceImport,
  type ProductPriceImportPreview,
} from '@/app/(protected)/settings/prices/actions';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { useFormatters } from '@/lib/i18n/formatters';

interface PriceImportProps {
  branch: Branch;
}

export function PriceImport({ branch }: PriceImportProps) {
  const fmt = useFormatters();
  const t = useTranslations('priceImport');
  const tCommon = useTranslations('common');
  // Two resolvers: exporting words a 422 as "this branch has no prices yet", where the import
  // reads the same code as a file it could not use.
  const message = useApiErrorMessage('priceImport');
  const exportMessage = useApiErrorMessage('priceImport.export');
  const [file, setFile] = useState<File | null>(null);
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

  function onPreview() {
    if (!file) return;
    setError(null);
    setSuccessCount(null);
    const formData = new FormData();
    formData.set('file', file);
    startPreview(async () => {
      const result = await previewPriceImport(branch.id, formData);
      if (!result.ok) {
        setPreview(null);
        setError(message(result.error));
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
        setError(message(result.error));
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
        setError(exportMessage(result.error));
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
        onSubmit={(event) => {
          event.preventDefault();
          onPreview();
        }}
        noValidate
        className="flex flex-col gap-y-4"
      >
        <Dropzone
          accept=".xlsx,.csv"
          disabled={busy}
          onFile={(next) => setFile(next ?? null)}
          title={tCommon('fileUpload.title')}
          releaseLabel={tCommon('fileUpload.release')}
          chooseLabel={file ? tCommon('fileUpload.replace') : tCommon('fileUpload.choose')}
          hint={tCommon('fileUpload.spreadsheetFormats')}
          fileName={file?.name}
        />
        <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
          <PendingButton
            type="button"
            variant="outline"
            onClick={onExport}
            disabled={busy}
            pending={exporting}
            pendingLabel={t('export.submitting')}
          >
            {t('export.submit')}
          </PendingButton>
          <PendingButton
            type="submit"
            disabled={busy || !file}
            pending={previewing}
            pendingLabel={t('form.previewing')}
          >
            {t('form.preview')}
          </PendingButton>
        </div>
      </form>

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
          <div className="overflow-hidden border border-border rounded-1.5xl shadow-e1">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('table.row')}</TableHead>
                  <TableHead>{t('table.code')}</TableHead>
                  <TableHead>{t('table.product')}</TableHead>
                  <TableHead className="text-right">{t('table.currentPrice')}</TableHead>
                  <TableHead className="text-right">{t('table.newPrice')}</TableHead>
                  <TableHead className="text-right">{t('table.minPrice')}</TableHead>
                  <TableHead>{t('table.result')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {preview.rows.map((row) => (
                  <TableRow
                    key={`${row.rowNumber}-${row.code}`}
                    className={
                      row.errors.length > 0 ? 'bg-danger-subtle hover:bg-danger-subtle' : undefined
                    }
                  >
                    <TableCell>{row.rowNumber}</TableCell>
                    <TableCell className="text-paragraph-sm-medium">{row.code || '—'}</TableCell>
                    <TableCell>{row.productName || '—'}</TableCell>
                    <TableCell className="text-right tabular-nums">
                      {row.currentPrice ? fmt.currency(row.currentPrice, row.currency) : '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {row.price ? fmt.currency(row.price, row.currency) : '—'}
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {row.minPrice ? fmt.currency(row.minPrice, row.currency) : '—'}
                    </TableCell>
                    <TableCell>
                      {row.errors.length === 0 ? (
                        <span>{t('valid')}</span>
                      ) : (
                        <ul className="flex flex-col gap-y-1 text-paragraph-xs-medium text-danger-foreground">
                          {row.errors.map((rowError) => (
                            <li key={rowError}>{t(`rowErrors.${rowError}`)}</li>
                          ))}
                        </ul>
                      )}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          {!preview.canConfirm ? (
            <p className="text-paragraph-sm text-danger-foreground">{t('fixErrors')}</p>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}
