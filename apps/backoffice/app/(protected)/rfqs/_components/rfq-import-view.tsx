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
  Skeleton,
  Textarea,
} from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { SetupNotice } from '@/components/setup-notice';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { listChannels, type Channel } from '@/lib/api/channels';
import { errorCodeOf } from '@/lib/api/errors';
import { createFileRfqDraft } from '@/lib/api/rfqs-client';

// Mirrors the API's accepted attachment types, so a file it would refuse is not offered. The legacy
// Excel type is left out: the API accepts it only for the CSVs Windows labels that way.
const ACCEPTED_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/heic',
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'text/csv',
  'audio/mpeg',
  'audio/mp4',
  'audio/ogg',
  'audio/wav',
  'audio/webm',
  'text/plain',
];

type ChannelList =
  | { status: 'loading' }
  | { status: 'failed' }
  | { status: 'ready'; items: Channel[] };

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
  const { isAdmin } = useRfqList();
  const [channels, setChannels] = useState<ChannelList>({ status: 'loading' });
  const [channelId, setChannelId] = useState<string | null>(null);
  const [client, setClient] = useState('');
  const [note, setNote] = useState('');
  const [processing, startProcessing] = useTransition();

  // Only a sole channel is preselected, and a choice made for one branch means nothing in the next.
  useEffect(() => {
    setChannelId(null);
    if (!activeBranchId) return;
    let cancelled = false;
    setChannels({ status: 'loading' });
    (async () => {
      try {
        const items = await listChannels(activeBranchId);
        if (cancelled) return;
        setChannels({ status: 'ready', items });
        if (items.length === 1) setChannelId(items[0]?.id ?? null);
      } catch {
        if (!cancelled) setChannels({ status: 'failed' });
      }
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

  function channelLabel(channel: Channel) {
    const type = tChannel(channel.type.toLowerCase());
    return channel.identifier ? `${type} · ${channel.identifier}` : type;
  }

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
      {activeBranchId && channels.status === 'ready' && channels.items.length === 0 ? (
        <SetupNotice issue="NO_INTAKE_CHANNELS" isAdmin={isAdmin} />
      ) : null}

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
        {/* With no channel the notice above says why; there is nothing to show here. */}
        {channels.status === 'ready' && channels.items.length === 0 ? null : (
          <div className="flex flex-col gap-y-1">
            {/* A label names a control, and only a choice between several channels has one. */}
            {channels.status === 'ready' && channels.items.length > 1 ? (
              <Label htmlFor="rfq-import-channel">{t('channelLabel')}</Label>
            ) : (
              <span className="text-paragraph-sm-medium text-foreground">{t('channelLabel')}</span>
            )}
            {!activeBranchId ? null : channels.status === 'loading' ? (
              <div role="status">
                <Skeleton className="h-9 w-full rounded-lg" />
                <span className="sr-only">{t('channelsLoading')}</span>
              </div>
            ) : channels.status === 'failed' ? (
              <p className="flex h-9 items-center text-paragraph-sm text-foreground-muted">
                {t('channelsFailed')}
              </p>
            ) : channels.items.length === 1 && channels.items[0] ? (
              <>
                <p className="flex h-9 items-center text-paragraph-sm text-foreground">
                  {channelLabel(channels.items[0])}
                </p>
                <p className="text-paragraph-xs text-foreground-muted">{t('channelOnly')}</p>
              </>
            ) : (
              <Combobox
                id="rfq-import-channel"
                options={channels.items.map((channel) => ({
                  value: channel.id,
                  label: channelLabel(channel),
                }))}
                value={channelId ?? ''}
                onValueChange={setChannelId}
                placeholder={t('channelPlaceholder')}
                aria-label={t('channelLabel')}
              />
            )}
          </div>
        )}

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
