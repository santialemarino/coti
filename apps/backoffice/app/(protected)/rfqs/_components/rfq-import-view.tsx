'use client';

import { useEffect, useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { FileTextIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Combobox,
  DialogFooter,
  Dropzone,
  Input,
  Label,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
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
  /* Lets the dialog refuse a click outside once there is work a stray click would throw away. */
  onDirtyChange?: (dirty: boolean) => void;
}

/*
 * The "importar archivo" step of creating a pedido: the seller drops the file the client sent —
 * a photo of a handwritten list, a PDF, a spreadsheet, a voice note — and the AI reads the
 * materials out of it. The draft it produces is a proposal: the seller lands on the order to
 * review every line before anything is priced.
 */
export function RfqImportView({
  onBack,
  onClose,
  onCreated,
  activeBranchId,
  onDirtyChange,
}: RfqImportViewProps) {
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
  // What a stray click outside the dialog would throw away.
  const dirty = file !== null || client.trim() !== '' || note.trim() !== '';

  useEffect(() => onDirtyChange?.(dirty), [dirty, onDirtyChange]);

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

      <Dropzone
        accept={ACCEPTED_TYPES.join(',')}
        disabled={processing}
        onFile={(chosen) => setFile(chosen ?? null)}
        icon={file ? FileTextIcon : undefined}
        title={t('dropzone.title')}
        releaseLabel={t('dropzone.release')}
        chooseLabel={file ? t('dropzone.replace') : t('dropzone.choose')}
        hint={t('acceptHint')}
        fileName={file?.name}
        fileMeta={
          file ? t('dropzone.selected', { size: Math.max(1, Math.round(file.size / 1024)) }) : null
        }
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-y-1">
          <Label htmlFor="rfq-import-channel">{t('channelLabel')}</Label>
          <Combobox
            id="rfq-import-channel"
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

        <div className="flex flex-col gap-y-1">
          <Label htmlFor="rfq-import-client">{t('clientLabel')}</Label>
          <Input
            id="rfq-import-client"
            value={client}
            onChange={(event) => setClient(event.target.value)}
            placeholder={t('clientPlaceholder')}
            maxLength={255}
          />
        </div>
      </div>

      <div className="flex flex-col gap-y-1">
        <Label htmlFor="rfq-import-note">{t('noteLabel')}</Label>
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
