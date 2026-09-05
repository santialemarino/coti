'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Button, DialogFooter, PendingButton } from '@repo/ui/components';

interface RfqImportViewProps {
  onBack: () => void;
  onClose: () => void;
}

/*
 * The "importar archivo" step of creating a pedido: the seller drops a file (PDF, Excel, photo,
 * audio, ...) and the AI reads it and proposes the order. The ingestion endpoint does not exist
 * yet, so the processing is simulated with a latency, the way the RFQ list mocks its data — a real
 * action is a change of this component alone.
 */
export function RfqImportView({ onBack, onClose }: RfqImportViewProps) {
  const t = useTranslations('rfqs.create.import');
  const tToast = useTranslations('rfqs.create.toast');
  const [file, setFile] = useState<File | null>(null);
  const [processing, startProcessing] = useTransition();

  function onSubmit() {
    if (!file) return;
    startProcessing(async () => {
      await new Promise((resolve) => setTimeout(resolve, 900));
      toast.success(tToast('imported', { filename: file.name }));
      onClose();
    });
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
      noValidate
      className="flex flex-col gap-y-5"
    >
      <div className="flex flex-col gap-y-1">
        <label htmlFor="rfq-import-file" className="text-paragraph-sm-medium">
          {t('fileLabel')}
        </label>
        <input
          id="rfq-import-file"
          type="file"
          required
          accept=".pdf,.xlsx,.xls,.csv,.txt,image/*,audio/*"
          onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          className="h-10 px-3 py-2 bg-background border border-input rounded-lg outline-none transition-[border-color,box-shadow] duration-200 ease-out-soft focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 file:mr-3 file:border-0 file:bg-transparent file:text-paragraph-sm-medium"
        />
      </div>
      <p className="text-paragraph-sm text-foreground-muted">{t('acceptHint')}</p>

      <DialogFooter>
        <Button type="button" variant="outline" disabled={processing} onClick={onBack}>
          {t('back')}
        </Button>
        <PendingButton
          type="submit"
          disabled={!file}
          pending={processing}
          pendingLabel={t('processing')}
        >
          {t('submit')}
        </PendingButton>
      </DialogFooter>
    </form>
  );
}
