'use client';

import * as React from 'react';
import { DownloadIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { PendingButton } from '@repo/ui/components';
import { throwApiError } from '@/lib/api/errors';

function filenameFrom(disposition: string | null): string {
  const match = disposition?.match(/filename="?([^";]+)"?/i);
  return match?.[1] ?? 'reporte.pdf';
}

export function ReportDownloadButton({ from, to }: { from: string; to: string }) {
  const t = useTranslations('reports.download');
  const [pending, setPending] = React.useState(false);

  async function download() {
    setPending(true);
    try {
      const query = new URLSearchParams({ from, to });
      const response = await fetch(`/api/reports/pdf?${query.toString()}`, { cache: 'no-store' });
      if (!response.ok) await throwApiError(response);

      const url = URL.createObjectURL(await response.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = filenameFrom(response.headers.get('Content-Disposition'));
      document.body.append(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch {
      toast.error(t('error'));
    } finally {
      setPending(false);
    }
  }

  return (
    <PendingButton
      type="button"
      variant="outline"
      pending={pending}
      pendingLabel={t('pending')}
      onClick={download}
    >
      <DownloadIcon aria-hidden="true" />
      {t('label')}
    </PendingButton>
  );
}
