'use client';

import { useEffect, useState, useTransition } from 'react';
import { CheckCircle2Icon, DownloadIcon, FileSpreadsheetIcon, UploadCloudIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Button,
  Callout,
  Card,
  PendingButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import {
  confirmCatalogImport,
  downloadCatalogTemplate,
  previewCatalogImport,
  type CatalogImportPreview,
} from '@/app/(protected)/_actions/catalog-import';
import { FileDropzone } from '@/components/file-dropzone';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { useFormatters } from '@/lib/i18n/formatters';

interface CatalogUploadProps {
  branch: Branch;
  formId?: string;
  submitPlacement?: 'inline' | 'external';
  onBusyChange?: (busy: boolean) => void;
  onPreview: (preview: CatalogImportPreview) => void | Promise<void>;
}

export function CatalogUpload({
  branch,
  formId,
  submitPlacement = 'inline',
  onBusyChange,
  onPreview,
}: CatalogUploadProps) {
  const t = useTranslations('catalogImport');
  const message = useApiErrorMessage('catalogImport');
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [previewing, startPreview] = useTransition();
  const [downloading, startDownload] = useTransition();
  const busy = previewing || downloading;

  useEffect(() => onBusyChange?.(busy), [busy, onBusyChange]);
  useEffect(() => () => onBusyChange?.(false), [onBusyChange]);

  function choose(next: File | undefined) {
    setError(null);
    setFile(next ?? null);
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
      await onPreview(result.preview);
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

      <form id={formId} onSubmit={onSubmit} noValidate className="flex flex-col gap-y-4">
        <FileDropzone accept=".xlsx,.csv" disabled={busy} onFile={choose} className="min-h-56">
          {({ dragging, openFileDialog }) => (
            <>
              <span
                data-dragging={dragging}
                className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground transition-[scale,translate] duration-200 ease-out-soft data-[dragging=true]:scale-110 data-[dragging=true]:-translate-y-1"
              >
                {file ? (
                  <FileSpreadsheetIcon aria-hidden="true" className="size-6" />
                ) : (
                  <UploadCloudIcon aria-hidden="true" className="size-6" />
                )}
              </span>
              <div className="flex flex-col items-center gap-y-1 text-center">
                <p className="break-all text-paragraph-medium">
                  {dragging ? t('dropzone.release') : file ? file.name : t('dropzone.title')}
                </p>
                <p className="text-paragraph-sm text-foreground-muted">
                  {file
                    ? t('dropzone.selected', { size: Math.max(1, Math.round(file.size / 1024)) })
                    : t('dropzone.hint')}
                </p>
              </div>
              <Button type="button" variant="outline" disabled={busy} onClick={openFileDialog}>
                {file ? t('dropzone.replace') : t('dropzone.choose')}
              </Button>
            </>
          )}
        </FileDropzone>

        {error ? <Callout tone="danger">{error}</Callout> : null}

        <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
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
          {submitPlacement === 'inline' ? (
            <PendingButton
              type="submit"
              disabled={busy || !file}
              pending={previewing}
              pendingLabel={t('previewing')}
            >
              {t('continue')}
            </PendingButton>
          ) : null}
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

      <div className="overflow-hidden border rounded-1.5xl shadow-e1">
        <Table>
          <TableHeader>
            <tr>
              <TableHead>{t('table.row')}</TableHead>
              <TableHead>{t('table.code')}</TableHead>
              <TableHead>{t('table.product')}</TableHead>
              <TableHead>{t('table.family')}</TableHead>
              <TableHead>{t('table.price')}</TableHead>
              <TableHead>{t('table.result')}</TableHead>
            </tr>
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
                <TableCell>{row.name || '—'}</TableCell>
                <TableCell>{row.family || '—'}</TableCell>
                <TableCell>{row.price ? fmt.currency(row.price) : '—'}</TableCell>
                <TableCell>
                  {row.errors.length === 0 ? (
                    <Badge tone="success">
                      <CheckCircle2Icon aria-hidden="true" />
                      {t('valid')}
                    </Badge>
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

      <div className="flex flex-col items-stretch gap-3 sm:flex-row sm:items-center sm:justify-between">
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
    <Card className="p-4 gap-y-1 rounded-lg shadow-e1">
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
    </Card>
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
