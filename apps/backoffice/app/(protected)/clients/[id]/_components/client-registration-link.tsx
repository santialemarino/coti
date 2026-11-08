'use client';

import { useState } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import { Button, ConfirmDialog } from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf, throwApiError } from '@/lib/api/errors';

interface ClientRegistrationLinkProps {
  clientId: string;
  phone: string | null;
  email: string | null;
}

export function ClientRegistrationLink({ clientId, phone, email }: ClientRegistrationLinkProps) {
  const t = useTranslations('clients.registration');
  const message = useApiErrorMessage('clients.registration');
  const { activeBranchId } = useRfqList();
  const [selected, setSelected] = useState<{ kind: 'EMAIL' | 'WHATSAPP'; value: string } | null>(
    null,
  );
  const [pending, setPending] = useState(false);
  async function send() {
    if (!selected || !activeBranchId || pending) return;
    setPending(true);
    try {
      const response = await fetch(
        `/api/clients/${encodeURIComponent(clientId)}/registration-link`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Branch-Id': activeBranchId },
          body: JSON.stringify({ kind: selected.kind }),
        },
      );
      if (!response.ok) await throwApiError(response);
      toast.success(t('sent'));
      setSelected(null);
    } catch (cause) {
      toast.error(message(errorCodeOf(cause)));
    } finally {
      setPending(false);
    }
  }
  return (
    <>
      <div className="flex flex-wrap gap-2">
        {phone ? (
          <Button
            variant="outline"
            disabled={!activeBranchId}
            onClick={() => setSelected({ kind: 'WHATSAPP', value: phone })}
          >
            {t(activeBranchId ? 'whatsapp' : 'selectBranch')}
          </Button>
        ) : null}
        {email ? (
          <Button
            variant="outline"
            disabled={!activeBranchId}
            onClick={() => setSelected({ kind: 'EMAIL', value: email })}
          >
            {t(activeBranchId ? 'email' : 'selectBranch')}
          </Button>
        ) : null}
      </div>
      <ConfirmDialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open && !pending) setSelected(null);
        }}
        entity={selected}
        title={t('title')}
        description={(contact) => t('confirm', { contact: contact.value })}
        onConfirm={send}
        pending={pending}
        tone="default"
        labels={{ confirm: t('send'), pending: t('sending'), cancel: t('cancel') }}
      />
    </>
  );
}
