'use client';

import { useState, useTransition } from 'react';
import { MessageCircleIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Button, Callout, ConfirmDialog, PendingButton } from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { settleReplyAcceptance } from '@/lib/api/rfqs-client';

interface ReplyAcceptanceCalloutProps {
  quoteId: string;
  branchId: string;
  reference: string;
  // The acceptance still waits for the seller, so the sale cannot close yet.
  unconfirmed: boolean;
  // The sale is unpaid, uninvoiced and not waived; the API has the last word on the send.
  undoable: boolean;
  readOnly?: boolean;
  onChanged: () => Promise<void>;
}

/*
 * An acceptance read from the customer's reply. While it waits for the seller it offers «Confirmar»
 * and «Deshacer»; once confirmed, «Deshacer» stays while nothing has closed the sale.
 */
export function ReplyAcceptanceCallout({
  quoteId,
  branchId,
  reference,
  unconfirmed,
  undoable,
  readOnly = false,
  onChanged,
}: ReplyAcceptanceCalloutProps) {
  const t = useTranslations('rfqs.detail.callouts.replyAcceptance');
  const tCommon = useTranslations('common');
  const errorMessage = useApiErrorMessage('rfqs.detail.lifecycle');
  const [confirming, startConfirm] = useTransition();
  const [undoOpen, setUndoOpen] = useState(false);
  const [undoing, setUndoing] = useState(false);

  const canUndo = !readOnly && (unconfirmed || undoable);

  function handleConfirm() {
    startConfirm(async () => {
      try {
        await settleReplyAcceptance(quoteId, branchId, 'confirm');
        toast.success(t('confirmSuccess', { reference }));
        await onChanged();
      } catch (error) {
        toast.error(errorMessage(errorCodeOf(error)));
      }
    });
  }

  async function handleUndo() {
    setUndoing(true);
    try {
      await settleReplyAcceptance(quoteId, branchId, 'undo');
      toast.success(t('undoSuccess', { reference }));
      setUndoOpen(false);
      await onChanged();
    } catch (error) {
      toast.error(errorMessage(errorCodeOf(error)));
      setUndoOpen(false);
    } finally {
      setUndoing(false);
    }
  }

  return (
    <>
      <Callout
        tone={unconfirmed ? 'warning' : 'success'}
        icon={MessageCircleIcon}
        title={t(unconfirmed ? 'unconfirmedTitle' : 'confirmedTitle')}
        action={
          canUndo ? (
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={confirming}
                onClick={() => setUndoOpen(true)}
              >
                {t('undo')}
              </Button>
              {unconfirmed ? (
                <PendingButton
                  type="button"
                  size="sm"
                  onClick={handleConfirm}
                  pending={confirming}
                  pendingLabel={t('confirming')}
                >
                  {t('confirm')}
                </PendingButton>
              ) : null}
            </div>
          ) : null
        }
      >
        {t(unconfirmed ? 'unconfirmed' : 'confirmed')}
      </Callout>

      <ConfirmDialog
        open={undoOpen}
        onOpenChange={(open) => !undoing && setUndoOpen(open)}
        entity={reference}
        title={t('undoTitle')}
        description={(entity) => t('undoDescription', { reference: entity })}
        onConfirm={handleUndo}
        pending={undoing}
        tone="danger"
        labels={{
          confirm: t('undoConfirm'),
          pending: t('undoing'),
          cancel: tCommon('actions.cancel'),
        }}
      />
    </>
  );
}
