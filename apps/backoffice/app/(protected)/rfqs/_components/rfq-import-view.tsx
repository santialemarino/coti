'use client';

import { useEffect, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { FileTextIcon, UploadCloudIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Combobox,
  DialogFooter,
  Input,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
import { FileDropzone } from '@/components/file-dropzone';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { listChannels, type Channel } from '@/lib/api/channels';
import { errorCodeOf } from '@/lib/api/errors';
import { createFileRfqDraft } from '@/lib/api/rfqs-client';

// Mirrors the API's accepted attachment types, so a file it would refuse is not offered.
const ACCEPTED_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/heic',
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.ms-excel',
  'text/csv',
  'audio/mpeg',
  'audio/mp4',
  'audio/ogg',
  'audio/wav',
  'audio/webm',
  'text/plain',
];

interface RfqImportViewProps {
  onBack: () => void;
  onClose: () => void;
  onCreated: () => void;
  activeBranchId: string | null;
}

/*
 * The "importar archivo" step of creating a pedido: the seller drops the file the client sent —
 * a photo of a handwritten list, a PDF, a spreadsheet, a voice note — and the AI reads the
 * materials out of it. The draft it produces is a proposal: the seller lands on the order to
 * review every line before anything is priced.
 */
export function RfqImportView({ onBack, onClose, onCreated, activeBranchId }: RfqImportViewProps) {
  const router = useRouter();
  const t = useTranslations('rfqs.create.import');
  const tToast = useTranslations('rfqs.create.toast');
  const tChannel = useTranslations('rfqs.channels');
  const message = useApiErrorMessage('rfqs.create.import');

  const [file, setFile] = useState<File | null>(null);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [channelId, setChannelId] = useState<string | null>(null);
  const [client, setClient] = useState('');
  const [note, setNote] = useState('');
  const [processing, startProcessing] = useTransition();

  /* The order has to name the channel it arrived through, so the branch's open ones are offered. */
  useEffect(() => {
    if (!activeBranchId) return;
    let cancelled = false;
    (async () => {
      const results = await listChannels(activeBranchId);
      if (cancelled) return;
      setChannels(results);
      setChannelId((current) => current ?? results[0]?.id ?? null);
    })();
    return () => {
      cancelled = true;
    };
  }, [activeBranchId]);

  // A new order has no branch of its own to fall back on, so one has to be selected before it
  // can be created at all — the same gate the manual flow applies.
  const canSubmit = !!file && !!channelId && !!activeBranchId;

  function onSubmit() {
    if (!file || !channelId) return;
    startProcessing(async () => {
      try {
        const draft = await createFileRfqDraft(
          file,
          {
            channelId,
            clientLabel: client.trim() || undefined,
            note: note.trim() || undefined,
          },
          activeBranchId,
        );
        // No quote means the model read no material out of the file. The order and the file are
        // kept either way, so the seller is sent to it rather than left with nothing.
        toast[draft.quote ? 'success' : 'warning'](
          draft.quote ? tToast('imported', { count: draft.items.length }) : tToast('importedEmpty'),
        );
        onCreated();
        onClose();
        router.push(ROUTES.rfqsDetail(draft.rfq.id));
      } catch (error) {
        toast.error(message(errorCodeOf(error)));
      }
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
      {activeBranchId ? null : <Callout tone="warning">{t('noBranch')}</Callout>}

      <FileDropzone
        accept={ACCEPTED_TYPES.join(',')}
        disabled={processing}
        onFile={(chosen) => setFile(chosen ?? null)}
      >
        {({ dragging, openFileDialog }) => (
          <>
            <span
              data-dragging={dragging}
              className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground transition-[scale,translate] duration-200 ease-out-soft data-[dragging=true]:scale-110 data-[dragging=true]:-translate-y-1"
            >
              {file ? (
                <FileTextIcon aria-hidden="true" className="size-6" />
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
                  : t('acceptHint')}
              </p>
            </div>
            <Button type="button" variant="outline" disabled={processing} onClick={openFileDialog}>
              {file ? t('dropzone.replace') : t('dropzone.choose')}
            </Button>
          </>
        )}
      </FileDropzone>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-y-1.5">
          <label htmlFor="rfq-import-channel" className="text-paragraph-sm-medium">
            {t('channelLabel')}
          </label>
          <Combobox
            options={channels.map((channel) => ({
              value: channel.id,
              label: channel.identifier
                ? `${tChannel(channel.type.toLowerCase())} · ${channel.identifier}`
                : tChannel(channel.type.toLowerCase()),
            }))}
            value={channelId ?? ''}
            onValueChange={setChannelId}
            placeholder={t('channelPlaceholder')}
            emptyLabel={t('channelEmpty')}
            aria-label={t('channelLabel')}
          />
        </div>

        <div className="flex flex-col gap-y-1.5">
          <label htmlFor="rfq-import-client" className="text-paragraph-sm-medium">
            {t('clientLabel')}
          </label>
          <Input
            id="rfq-import-client"
            value={client}
            onChange={(event) => setClient(event.target.value)}
            placeholder={t('clientPlaceholder')}
            maxLength={255}
          />
        </div>
      </div>

      <div className="flex flex-col gap-y-1.5">
        <label htmlFor="rfq-import-note" className="text-paragraph-sm-medium">
          {t('noteLabel')}
        </label>
        <Textarea
          id="rfq-import-note"
          value={note}
          onChange={(event) => setNote(event.target.value)}
          placeholder={t('notePlaceholder')}
          rows={2}
        />
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" disabled={processing} onClick={onBack}>
          {t('back')}
        </Button>
        <PendingButton
          type="submit"
          disabled={!canSubmit}
          pending={processing}
          pendingLabel={t('processing')}
        >
          {t('submit')}
        </PendingButton>
      </DialogFooter>
    </form>
  );
}
