'use client';

import { useState } from 'react';
import {
  CheckCircle2Icon,
  PencilLineIcon,
  RotateCcwIcon,
  SendIcon,
  XCircleIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  ConfirmDialog,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Spinner,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import type { QuoteReactivationMode } from '@/lib/api/rfqs';
import { reactivateQuote, transitionQuote } from '@/lib/api/rfqs-client';

type ClosureStatus = 'ACCEPTED' | 'REJECTED';

interface QuoteLifecycleActionsProps {
  quoteId: string;
  branchId: string;
  status: string;
  onChanged: () => Promise<void>;
}

export function QuoteLifecycleActions({
  quoteId,
  branchId,
  status,
  onChanged,
}: QuoteLifecycleActionsProps) {
  const t = useTranslations('rfqs.detail.lifecycle');
  const tCommon = useTranslations('common');
  const errorMessage = useApiErrorMessage('rfqs.detail.lifecycle.errors');
  const [closure, setClosure] = useState<ClosureStatus | null>(null);
  const [closing, setClosing] = useState(false);
  const [reactivateOpen, setReactivateOpen] = useState(false);
  const [reactivating, setReactivating] = useState<QuoteReactivationMode | null>(null);

  const canClose = status === 'SENT';
  const canReactivate = status === 'ACCEPTED' || status === 'REJECTED';
  const closureKey = closure === 'ACCEPTED' ? 'accept' : 'reject';

  async function handleClosure() {
    if (!closure) return;
    setClosing(true);
    try {
      await transitionQuote(quoteId, branchId, closure);
      toast.success(t(`${closureKey}.success`));
      setClosure(null);
      await onChanged();
    } catch (error) {
      toast.error(errorMessage(errorCodeOf(error)));
    } finally {
      setClosing(false);
    }
  }

  async function handleReactivate(mode: QuoteReactivationMode) {
    setReactivating(mode);
    try {
      await reactivateQuote(quoteId, branchId, mode);
      toast.success(t(`reactivate.${mode === 'RESEND' ? 'resend' : 'edit'}.success`));
      setReactivateOpen(false);
      await onChanged();
    } catch (error) {
      toast.error(errorMessage(errorCodeOf(error)));
    } finally {
      setReactivating(null);
    }
  }

  if (!canClose && !canReactivate) return null;

  return (
    <>
      {canClose ? (
        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => setClosure('REJECTED')}>
            <XCircleIcon />
            {t('reject.button')}
          </Button>
          <Button type="button" onClick={() => setClosure('ACCEPTED')}>
            <CheckCircle2Icon />
            {t('accept.button')}
          </Button>
        </div>
      ) : (
        <Button type="button" variant="outline" onClick={() => setReactivateOpen(true)}>
          <RotateCcwIcon />
          {t('reactivate.button')}
        </Button>
      )}

      <ConfirmDialog
        open={closure !== null}
        onOpenChange={(open) => !open && !closing && setClosure(null)}
        entity={closure}
        pending={closing}
        title={t(`${closureKey}.title`)}
        description={() => t(`${closureKey}.description`)}
        onConfirm={handleClosure}
        tone={closure === 'REJECTED' ? 'danger' : 'default'}
        labels={{
          confirm: t(`${closureKey}.confirm`),
          pending: t(`${closureKey}.pending`),
          cancel: tCommon('actions.cancel'),
        }}
      />

      <Dialog
        open={reactivateOpen}
        onOpenChange={(open) => !reactivating && setReactivateOpen(open)}
      >
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!reactivating}>
          <DialogHeader>
            <DialogTitle>{t('reactivate.title')}</DialogTitle>
            <DialogDescription>{t('reactivate.description')}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-y-3">
            <Button
              type="button"
              variant="outline"
              className="h-auto w-full justify-start px-4 py-3 whitespace-normal"
              disabled={reactivating !== null}
              onClick={() => void handleReactivate('RESEND')}
            >
              {reactivating === 'RESEND' ? (
                <Spinner size="sm" label={t('reactivate.resend.title')} />
              ) : (
                <SendIcon />
              )}
              <span className="flex min-w-0 flex-col items-start gap-y-0.5 text-left">
                <span className="text-paragraph-sm-medium">{t('reactivate.resend.title')}</span>
                <span className="text-paragraph-xs text-foreground-muted">
                  {t('reactivate.resend.description')}
                </span>
              </span>
            </Button>

            <Button
              type="button"
              variant="outline"
              className="h-auto w-full justify-start px-4 py-3 whitespace-normal"
              disabled={reactivating !== null}
              onClick={() => void handleReactivate('EDIT')}
            >
              {reactivating === 'EDIT' ? (
                <Spinner size="sm" label={t('reactivate.edit.title')} />
              ) : (
                <PencilLineIcon />
              )}
              <span className="flex min-w-0 flex-col items-start gap-y-0.5 text-left">
                <span className="text-paragraph-sm-medium">{t('reactivate.edit.title')}</span>
                <span className="text-paragraph-xs text-foreground-muted">
                  {t('reactivate.edit.description')}
                </span>
              </span>
            </Button>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={reactivating !== null}
              onClick={() => setReactivateOpen(false)}
            >
              {tCommon('actions.cancel')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
