'use client';

import { useId, useOptimistic, useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  Switch,
} from '@repo/ui/components';
import { setInvoicingEnabled } from '@/app/(protected)/settings/invoicing/actions';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';

interface InvoicingSwitchCardProps {
  enabled: boolean;
}

/*
 * Whether the corralón invoices with Coti. Turning it on only reveals the ARCA connection, so it goes
 * straight through; turning it off hides invoicing everywhere, so it asks first.
 */
export function InvoicingSwitchCard({ enabled }: InvoicingSwitchCardProps) {
  const t = useTranslations('invoicing.settings.switch');
  const message = useApiErrorMessage('invoicing.settings.switch');
  const [shown, setShown] = useOptimistic(enabled);
  const [confirming, setConfirming] = useState(false);
  const [pending, startTransition] = useTransition();
  const descriptionId = useId();

  function save(next: boolean) {
    startTransition(async () => {
      setShown(next);
      const result = await setInvoicingEnabled(next);
      setConfirming(false);
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      if (next) toast.success(t(result.arcaConnected ? 'enabledConnected' : 'enabled'));
      else toast.success(t(result.hasInvoices ? 'disabledWithInvoices' : 'disabled'));
    });
  }

  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-x-4">
        <div className="flex flex-col gap-y-1.5">
          <CardTitle>{t('label')}</CardTitle>
          <CardDescription id={descriptionId}>{t('description')}</CardDescription>
        </div>
        <Switch
          checked={shown}
          disabled={pending}
          aria-label={t('label')}
          aria-describedby={descriptionId}
          onCheckedChange={(next) => (next ? save(true) : setConfirming(true))}
        />
      </CardHeader>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(open) => {
          if (!pending) setConfirming(open);
        }}
        entity={confirming ? t('label') : null}
        title={t('confirm.title')}
        description={() => t('confirm.description')}
        onConfirm={() => save(false)}
        pending={pending}
        tone="default"
        labels={{
          confirm: t('confirm.action'),
          pending: t('confirm.pending'),
          cancel: t('confirm.cancel'),
        }}
      />
    </Card>
  );
}
