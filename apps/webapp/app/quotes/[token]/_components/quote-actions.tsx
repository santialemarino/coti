'use client';

import { useState } from 'react';
import {
  CheckCircle2Icon,
  CheckIcon,
  CircleXIcon,
  MessageCircleIcon,
  PencilIcon,
  XIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Card,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Label,
  PendingButton,
  StatusScreen,
  Textarea,
} from '@repo/ui/components';
import type { CustomerActionType, PublicQuoteActionResult } from '@/lib/api/public-quotes';

const MAX_ACTION_MESSAGE = 512;
const PREVIEW_ACTION_DELAY_MS = 700;

type ActionChoice = CustomerActionType;

interface QuoteActionsProps {
  /* Absent on the dev-only preview route, which demos the flow without a wire call. */
  token?: string;
  customerStatus?: CustomerActionType;
}

const RESULT_TONE: Record<CustomerActionType, 'success' | 'info' | 'danger'> = {
  ACCEPT: 'success',
  REQUEST_CHANGE: 'info',
  REJECT: 'danger',
};

const RESULT_ICON = {
  ACCEPT: CheckCircle2Icon,
  REQUEST_CHANGE: MessageCircleIcon,
  REJECT: CircleXIcon,
} as const;

const DIALOG_TITLE_KEY: Record<ActionChoice, string> = {
  ACCEPT: 'acceptTitle',
  REQUEST_CHANGE: 'requestChangesTitle',
  REJECT: 'rejectTitle',
};

const DIALOG_DESCRIPTION_KEY: Record<ActionChoice, string> = {
  ACCEPT: 'acceptDescription',
  REQUEST_CHANGE: 'requestChangesDescription',
  REJECT: 'rejectDescription',
};

const RESULT_DESCRIPTION_KEY: Record<CustomerActionType, string> = {
  ACCEPT: 'resultAccept',
  REQUEST_CHANGE: 'resultRequestChange',
  REJECT: 'resultReject',
};

const RESPONDED_DESCRIPTION_KEY: Record<CustomerActionType, string> = {
  ACCEPT: 'respondedAccept',
  REQUEST_CHANGE: 'respondedRequestChange',
  REJECT: 'respondedReject',
};

/*
 * The customer's answer to the frozen quote. A change request requires a message so the seller
 * knows what to rework. The preview route simulates the round trip without a running API.
 */
export function QuoteActions({ token, customerStatus }: QuoteActionsProps) {
  const t = useTranslations('quote.actions');

  const [choice, setChoice] = useState<ActionChoice | null>(null);
  const [message, setMessage] = useState('');
  const [messageTouched, setMessageTouched] = useState(false);
  const [busy, setBusy] = useState(false);
  const [submitError, setSubmitError] = useState(false);
  const [result, setResult] = useState<PublicQuoteActionResult | null>(null);
  const messageMissing = message.trim() === '';
  const showMessageError = choice === 'REQUEST_CHANGE' && messageMissing && messageTouched;

  function openDialog(next: ActionChoice) {
    setChoice(next);
    setMessage('');
    setMessageTouched(false);
    setSubmitError(false);
  }

  function closeDialog() {
    if (busy) return;
    setChoice(null);
    setMessage('');
    setMessageTouched(false);
    setSubmitError(false);
  }

  async function handleConfirm() {
    if (choice === null) return;
    if (choice === 'REQUEST_CHANGE' && messageMissing) {
      setMessageTouched(true);
      return;
    }
    setBusy(true);
    setSubmitError(false);
    try {
      const outcome = token
        ? await submitAction(token, { type: choice, message: message.trim() || undefined })
        : await simulateAction(choice);
      setResult(outcome);
      closeDialog();
    } catch {
      setSubmitError(true);
    } finally {
      setBusy(false);
    }
  }

  if (result) {
    return (
      <section className="flex flex-col gap-y-4">
        <h2 className="text-heading-6 text-foreground">{t('heading')}</h2>
        <Card>
          <StatusScreen
            icon={RESULT_ICON[result.customerStatus]}
            tone={RESULT_TONE[result.customerStatus]}
            title={t('resultTitle')}
            description={t(RESULT_DESCRIPTION_KEY[result.customerStatus])}
          />
        </Card>
      </section>
    );
  }

  if (customerStatus) {
    return (
      <section className="flex flex-col gap-y-4">
        <h2 className="text-heading-6 text-foreground">{t('heading')}</h2>
        <Card>
          <StatusScreen
            icon={RESULT_ICON[customerStatus]}
            tone={RESULT_TONE[customerStatus]}
            title={t('respondedTitle')}
            description={t(RESPONDED_DESCRIPTION_KEY[customerStatus])}
          />
        </Card>
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-y-4">
      <div className="flex flex-col gap-y-1 text-center">
        <h2 className="text-heading-6 text-foreground">{t('heading')}</h2>
        <p className="text-paragraph text-foreground-muted">{t('intro')}</p>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-center sm:gap-x-3">
        <Button type="button" onClick={() => openDialog('ACCEPT')}>
          <CheckIcon />
          {t('acceptLabel')}
        </Button>
        <Button type="button" variant="outline" onClick={() => openDialog('REQUEST_CHANGE')}>
          <PencilIcon />
          {t('requestChangesLabel')}
        </Button>
        <Button type="button" variant="destructive" onClick={() => openDialog('REJECT')}>
          <XIcon />
          {t('rejectLabel')}
        </Button>
      </div>

      <Dialog open={choice !== null} onOpenChange={(open) => !open && closeDialog()}>
        <DialogContent className="sm:max-w-md" closeOnClickOutside={!busy} showCloseButton={!busy}>
          <DialogHeader>
            <DialogTitle>{choice ? t(DIALOG_TITLE_KEY[choice]) : ''}</DialogTitle>
            <DialogDescription>{choice ? t(DIALOG_DESCRIPTION_KEY[choice]) : ''}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-y-3">
            {choice === 'REQUEST_CHANGE' ? (
              <div className="flex flex-col gap-y-1.5">
                <Label htmlFor="action-message" required>
                  {t('messageLabel')}
                </Label>
                <Textarea
                  id="action-message"
                  value={message}
                  onChange={(event) => setMessage(event.target.value)}
                  placeholder={t('messagePlaceholder')}
                  aria-invalid={showMessageError}
                  maxLength={MAX_ACTION_MESSAGE}
                  autoFocus
                />
                {showMessageError ? (
                  <p className="text-paragraph-sm text-danger-foreground">{t('messageRequired')}</p>
                ) : null}
              </div>
            ) : null}
            {submitError ? (
              <p className="text-paragraph-sm text-danger-foreground">{t('errorSubmit')}</p>
            ) : null}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" disabled={busy} onClick={closeDialog}>
              {t('cancel')}
            </Button>
            <PendingButton
              type="button"
              variant={choice === 'REJECT' ? 'destructive' : 'default'}
              pending={busy}
              pendingLabel={t('sending')}
              onClick={handleConfirm}
            >
              {t('confirm')}
            </PendingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

async function submitAction(
  token: string,
  action: { type: CustomerActionType; message?: string },
): Promise<PublicQuoteActionResult> {
  const response = await fetch('/api/public-quote-action', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, type: action.type, message: action.message }),
  });
  if (!response.ok) throw new Error('public quote action refused');
  return (await response.json()) as PublicQuoteActionResult;
}

async function simulateAction(choice: CustomerActionType): Promise<PublicQuoteActionResult> {
  await new Promise((resolve) => setTimeout(resolve, PREVIEW_ACTION_DELAY_MS));
  return { customerStatus: choice, createdAt: new Date().toISOString() };
}
