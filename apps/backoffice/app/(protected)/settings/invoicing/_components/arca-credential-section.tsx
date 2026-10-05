'use client';

import { useState, useTransition } from 'react';
import { useRouter } from 'next/navigation';
import { FileKeyIcon, FileLockIcon, Trash2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Callout,
  ConfirmDialog,
  Dropzone,
  MetaList,
  PendingButton,
} from '@repo/ui/components';
import {
  deleteArcaCredentials,
  uploadArcaCredentials,
} from '@/app/(protected)/settings/invoicing/actions';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { formatCuit, type ArcaCredential } from '@/lib/api/invoicing';
import { useFormatters } from '@/lib/i18n/formatters';

const CERTIFICATE_ACCEPT = '.crt,.pem,.cer';
const PRIVATE_KEY_ACCEPT = '.key,.pem';

interface ArcaCredentialSectionProps {
  credential: ArcaCredential | null;
}

/*
 * The certificate ARCA knows the corralón by, and its private key. Uploading replaces whatever was
 * there; the key never comes back from the server, so the status is all this screen can show.
 */
export function ArcaCredentialSection({ credential }: ArcaCredentialSectionProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('invoicing.settings.credential');
  const message = useApiErrorMessage('invoicing.settings.credential');
  const [certificate, setCertificate] = useState<File | null>(null);
  const [privateKey, setPrivateKey] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [uploading, startUpload] = useTransition();
  const [deleting, startDelete] = useTransition();

  const expired = credential ? new Date(credential.expiresAt) <= new Date() : false;

  function upload() {
    if (!certificate || !privateKey) {
      setError(t('bothRequired'));
      return;
    }
    setError(null);
    const payload = new FormData();
    payload.set('certificate', certificate);
    payload.set('private_key', privateKey);
    startUpload(async () => {
      const result = await uploadArcaCredentials(payload);
      if (!result.credential) {
        setError(message(result.error));
        return;
      }
      toast.success(
        t('uploaded', {
          cuit: formatCuit(result.credential.cuit),
          date: fmt.date(result.credential.expiresAt),
        }),
      );
      setCertificate(null);
      setPrivateKey(null);
      router.refresh();
    });
  }

  function remove() {
    startDelete(async () => {
      const result = await deleteArcaCredentials();
      if (!result.ok) {
        setError(message(result.error));
        setConfirmingDelete(false);
        return;
      }
      toast.success(t('deleted'));
      setConfirmingDelete(false);
      router.refresh();
    });
  }

  return (
    <section className="flex flex-col gap-y-5">
      <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3">
        <div className="flex flex-col gap-y-1">
          <div className="flex items-center gap-x-2">
            <h2 className="text-heading-6">{t('title')}</h2>
            {credential ? (
              <Badge tone={expired ? 'danger' : 'success'} size="sm" dot>
                {t(expired ? 'expired' : 'loaded')}
              </Badge>
            ) : (
              <Badge tone="warning" size="sm" dot>
                {t('missing')}
              </Badge>
            )}
          </div>
          {credential ? (
            <MetaList
              items={[
                t('cuit', { cuit: formatCuit(credential.cuit) }),
                t(expired ? 'expiredOn' : 'expiresOn', { date: fmt.date(credential.expiresAt) }),
              ]}
            />
          ) : (
            <p className="text-paragraph-sm text-foreground-muted">{t('missingDescription')}</p>
          )}
        </div>
        {credential ? (
          <Button variant="outline" onClick={() => setConfirmingDelete(true)} disabled={uploading}>
            <Trash2Icon aria-hidden="true" />
            {t('delete.action')}
          </Button>
        ) : null}
      </div>

      <p className="text-paragraph-xs text-foreground-muted">{t('hint')}</p>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="flex flex-col gap-y-2">
          <span className="text-paragraph-sm-medium">{t('certificate.label')}</span>
          <Dropzone
            accept={CERTIFICATE_ACCEPT}
            onFile={(file) => setCertificate(file ?? null)}
            title={t('certificate.title')}
            releaseLabel={t('release')}
            chooseLabel={certificate ? t('replaceFile') : t('chooseFile')}
            hint={t('certificate.hint')}
            fileName={certificate?.name}
            icon={FileLockIcon}
            disabled={uploading}
          />
        </div>
        <div className="flex flex-col gap-y-2">
          <span className="text-paragraph-sm-medium">{t('privateKey.label')}</span>
          <Dropzone
            accept={PRIVATE_KEY_ACCEPT}
            onFile={(file) => setPrivateKey(file ?? null)}
            title={t('privateKey.title')}
            releaseLabel={t('release')}
            chooseLabel={privateKey ? t('replaceFile') : t('chooseFile')}
            hint={t('privateKey.hint')}
            fileName={privateKey?.name}
            icon={FileKeyIcon}
            disabled={uploading}
          />
        </div>
      </div>

      {error ? <Callout tone="danger">{error}</Callout> : null}

      <div className="flex justify-end">
        <PendingButton pending={uploading} pendingLabel={t('uploading')} onClick={upload}>
          {t(credential ? 'replace' : 'upload')}
        </PendingButton>
      </div>

      <ConfirmDialog
        open={confirmingDelete}
        onOpenChange={(open) => !deleting && setConfirmingDelete(open)}
        entity={credential}
        title={t('delete.title')}
        description={(current) => t('delete.description', { cuit: formatCuit(current.cuit) })}
        onConfirm={remove}
        pending={deleting}
        labels={{
          confirm: t('delete.confirm'),
          pending: t('delete.confirming'),
          cancel: t('delete.cancel'),
        }}
      />
    </section>
  );
}
