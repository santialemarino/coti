'use client';

import { useRef, useState, useTransition } from 'react';
import { DownloadIcon, FileSpreadsheetIcon, UploadCloudIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout, PendingButton } from '@repo/ui/components';
import {
  confirmCatalogImport,
  downloadCatalogTemplate,
  previewCatalogImport,
  type CatalogImportPreview,
} from '@/app/(protected)/_actions/catalog-import';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { useFormatters } from '@/lib/i18n/formatters';

interface CatalogUploadProps {
  branch: Branch;
  onPreview: (preview: CatalogImportPreview) => void;
}

export function CatalogUpload({ branch, onPreview }: CatalogUploadProps) {
  const t = useTranslations('catalogImport');
  const message = useApiErrorMessage('catalogImport');
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [previewing, startPreview] = useTransition();
  const [downloading, startDownload] = useTransition();
  const busy = previewing || downloading;

  function choose(next: File | undefined) {
    setError(null);
    setFile(next ?? null);
  }

  function onDrop(event: React.DragEvent<HTMLDivElement>) {
    event.preventDefault();
    choose(event.dataTransfer.files[0]);
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file) {
      setError(t('fileRequired'));
      return;
    }
    setError(null);
    startPreview(async () => {
      const formData = new FormData();
      formData.set('file', file);
      const result = await previewCatalogImport(branch.id, formData);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      onPreview(result.preview);
    });
  }

  function onDownload() {
    setError(null);
    startDownload(async () => {
      const result = await downloadCatalogTemplate(branch.id);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      downloadBase64(result.filename, result.contentBase64);
    });
  }

  return (
    <div className="flex flex-col gap-y-5">
      <div className="grid gap-4 md:grid-cols-3">
        {(['download', 'complete', 'upload'] as const).map((key, index) => (
          <div key={key} className="flex items-start p-4 gap-x-3 bg-muted border rounded-lg">
            <span className="flex size-7 shrink-0 items-center justify-center bg-primary rounded-full text-paragraph-sm-medium text-primary-foreground">
              {index + 1}
            </span>
            <div className="flex flex-col gap-y-1">
              <p className="text-paragraph-sm-medium">{t(`steps.${key}.title`)}</p>
              <p className="text-paragraph-xs text-foreground-muted">
                {t(`steps.${key}.description`)}
              </p>
            </div>
          </div>
        ))}
      </div>

      <form onSubmit={onSubmit} noValidate className="flex flex-col gap-y-4">
        <input
          ref={inputRef}
          type="file"
          accept=".xlsx,.csv"
          className="sr-only"
          onChange={(event) => choose(event.target.files?.[0])}
        />
        <div
          onDragOver={(event) => event.preventDefault()}
          onDrop={onDrop}
          className="flex flex-col min-h-56 items-center justify-center px-6 py-8 gap-y-4 bg-card border-2 border-dashed border-strong rounded-1.5xl"
        >
          <span className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground">
            {file ? (
              <FileSpreadsheetIcon aria-hidden="true" className="size-6" />
            ) : (
              <UploadCloudIcon aria-hidden="true" className="size-6" />
            )}
          </span>
          <div className="flex flex-col items-center gap-y-1 text-center">
            <p className="text-paragraph-medium">{file ? file.name : t('dropzone.title')}</p>
            <p className="text-paragraph-sm text-foreground-muted">
              {file
                ? t('dropzone.selected', { size: Math.max(1, Math.round(file.size / 1024)) })
                : t('dropzone.hint')}
            </p>
          </div>
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => inputRef.current?.click()}
          >
            {file ? t('dropzone.replace') : t('dropzone.choose')}
          </Button>
        </div>

        {error ? <Callout tone="danger">{error}</Callout> : null}

        <div className="flex flex-wrap items-center justify-between gap-3">
          <PendingButton
            type="button"
            variant="outline"
            disabled={busy}
            pending={downloading}
            pendingLabel={t('template.downloading')}
            onClick={onDownload}
          >
            <DownloadIcon aria-hidden="true" />
            {t('template.download')}
          </PendingButton>
          <PendingButton
            type="submit"
            disabled={busy || !file}
            pending={previewing}
            pendingLabel={t('previewing')}
          >
            {t('review')}
          </PendingButton>
        </div>
      </form>
    </div>
  );
}

interface CatalogReviewProps {
  preview: CatalogImportPreview;
  onBack: () => void;
  onConfirmed: (importedRows: number, skippedRows: number) => void;
}

export function CatalogReview({ preview, onBack, onConfirmed }: CatalogReviewProps) {
  const fmt = useFormatters();
  const t = useTranslations('catalogImport');
  const message = useApiErrorMessage('catalogImport');
  const [error, setError] = useState<string | null>(null);
  const [confirming, startConfirm] = useTransition();

  function onConfirm() {
    setError(null);
    startConfirm(async () => {
      const result = await confirmCatalogImport(preview);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      onConfirmed(result.importedRows, result.skippedRows);
    });
  }

  return (
    <div className="flex flex-col gap-y-5">
      <div className="grid gap-3 sm:grid-cols-3">
        <Summary label={t('summary.total')} value={preview.rows.length} />
        <Summary label={t('summary.valid')} value={preview.validRows} tone="success" />
        <Summary label={t('summary.invalid')} value={preview.invalidRows} tone="danger" />
      </div>

      {preview.invalidRows > 0 ? (
        <Callout tone="warning">{t('invalidRowsSkipped', { count: preview.invalidRows })}</Callout>
      ) : null}
      {error ? <Callout tone="danger">{error}</Callout> : null}

      <div className="overflow-x-auto border rounded-lg">
        <table className="w-full border-collapse text-paragraph-sm">
          <thead className="bg-muted">
            <tr>
              <th className="px-3 py-2 text-left">{t('table.row')}</th>
              <th className="px-3 py-2 text-left">{t('table.code')}</th>
              <th className="px-3 py-2 text-left">{t('table.product')}</th>
              <th className="px-3 py-2 text-left">{t('table.family')}</th>
              <th className="px-3 py-2 text-left">{t('table.price')}</th>
              <th className="px-3 py-2 text-left">{t('table.result')}</th>
            </tr>
          </thead>
          <tbody>
            {preview.rows.map((row) => (
              <tr key={`${row.rowNumber}-${row.code}`} className="border-t">
                <td className="px-3 py-2">{row.rowNumber}</td>
                <td className="px-3 py-2 text-paragraph-sm-medium">{row.code || '—'}</td>
                <td className="px-3 py-2">{row.name || '—'}</td>
                <td className="px-3 py-2">{row.family || '—'}</td>
                <td className="px-3 py-2">{row.price ? fmt.currency(row.price) : '—'}</td>
                <td className="px-3 py-2">
                  {row.errors.length === 0 ? (
                    <span className="text-success-foreground">{t('valid')}</span>
                  ) : (
                    <ul className="flex flex-col gap-y-1 text-danger-foreground">
                      {row.errors.map((rowError) => (
                        <li key={rowError}>{t(`rowErrors.${rowError}`)}</li>
                      ))}
                    </ul>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between gap-x-3">
        <Button type="button" variant="outline" disabled={confirming} onClick={onBack}>
          {t('chooseAnother')}
        </Button>
        {preview.canConfirm ? (
          <PendingButton
            type="button"
            pending={confirming}
            pendingLabel={t('confirming')}
            onClick={onConfirm}
          >
            {t('confirm', { count: preview.validRows })}
          </PendingButton>
        ) : (
          <p className="text-paragraph-sm text-danger-foreground">{t('noValidRows')}</p>
        )}
      </div>
    </div>
  );
}

interface CatalogImportProps {
  branch: Branch;
}

export function CatalogImport({ branch }: CatalogImportProps) {
  const t = useTranslations('catalogImport');
  const [preview, setPreview] = useState<CatalogImportPreview | null>(null);
  const [result, setResult] = useState<{ imported: number; skipped: number } | null>(null);

  if (result) {
    return <Callout tone="success">{t('success', result)}</Callout>;
  }
  if (preview) {
    return (
      <CatalogReview
        preview={preview}
        onBack={() => setPreview(null)}
        onConfirmed={(imported, skipped) => setResult({ imported, skipped })}
      />
    );
  }
  return <CatalogUpload branch={branch} onPreview={setPreview} />;
}

interface SummaryProps {
  label: string;
  value: number;
  tone?: 'success' | 'danger';
}

function Summary({ label, value, tone }: SummaryProps) {
  return (
    <div className="flex flex-col p-4 gap-y-1 bg-card border rounded-lg">
      <span
        className={
          tone === 'success'
            ? 'text-heading-4 text-success-foreground'
            : tone === 'danger'
              ? 'text-heading-4 text-danger-foreground'
              : 'text-heading-4'
        }
      >
        {value}
      </span>
      <span className="text-paragraph-xs text-foreground-muted">{label}</span>
    </div>
  );
}

function downloadBase64(filename: string, contentBase64: string) {
  const binary = atob(contentBase64);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  const url = URL.createObjectURL(
    new Blob([bytes], {
      type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    }),
  );
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}
