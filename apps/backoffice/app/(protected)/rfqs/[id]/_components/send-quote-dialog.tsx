'use client';

import { useState } from 'react';
import { MailIcon, MessageCircleIcon, SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Checkbox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  PendingButton,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import {
  formatRfqReference,
  type QuoteDeliveryResponse,
  type RfqDetailResponse,
} from '@/lib/api/rfqs';
import { sendQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

// Same shape the API enforces, so a malformed number is caught before the round trip.
const E164 = /^\+[1-9]\d{7,14}$/;

interface SendQuoteDialogProps {
  detail: RfqDetailResponse;
  branchId: string;
  onSent: () => Promise<void>;
}

/*
 * Delivery of an approved quote. WhatsApp always carries the public link, and the email copy
 * is an independent extra rather than an alternative — the backend attempts each on its own
 * and reports both, so the dialog reports both too.
 */
export function SendQuoteDialog({ detail, branchId, onSent }: SendQuoteDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.send');
  const message = useApiErrorMessage('rfqs.detail.send');
  const [open, setOpen] = useState(false);
  const [phone, setPhone] = useState('');
  const [alsoEmail, setAlsoEmail] = useState(false);
  const [email, setEmail] = useState('');
  const [sending, setSending] = useState(false);

  const quoteId = detail.quote?.id ?? null;
  const phoneValid = E164.test(phone.trim());
  const emailValid = !alsoEmail || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
  const canSend = !!quoteId && phoneValid && emailValid;

  function handleOpenChange(next: boolean) {
    if (sending) return;
    setOpen(next);
  }

  async function handleSend() {
    if (!quoteId || !canSend) return;
    setSending(true);
    try {
      const result = await sendQuote(quoteId, branchId, {
        recipient_phone: phone.trim(),
        email_delivery: alsoEmail ? { address: email.trim() } : null,
      });
      reportOutcome(result.deliveries);
      setOpen(false);
      await onSent();
    } catch (error) {
      toast.error(message(errorCodeOf(error)));
    } finally {
      setSending(false);
    }
  }

  /*
   * A partly delivered send is not a failure and not a success: the seller has to know which
   * channel to follow up by hand, so each one is named rather than summed into one verdict.
   */
  function reportOutcome(deliveries: QuoteDeliveryResponse[]) {
    const failed = deliveries.filter((delivery) => delivery.tracking_status === 'FAILED');
    if (failed.length === 0) {
      toast.success(t('toast.sent'));
      return;
    }
    if (failed.length === deliveries.length) {
      toast.error(t('toast.failed'));
      return;
    }
    toast.warning(
      t('toast.partial', { channels: fmt.list(failed.map((delivery) => delivery.channel)) }),
    );
  }

  return (
    <>
      <Button type="button" onClick={() => handleOpenChange(true)}>
        <SendIcon className="size-4" />
        {t('button')}
      </Button>

      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-y-4">
            <div className="flex flex-col gap-y-1.5">
              <Label htmlFor="send-phone" required>
                <MessageCircleIcon aria-hidden="true" className="size-4 text-foreground-muted" />
                {t('phoneLabel')}
              </Label>
              <Input
                id="send-phone"
                type="tel"
                inputMode="tel"
                value={phone}
                onChange={(event) => setPhone(event.target.value)}
                placeholder={t('phonePlaceholder')}
                autoFocus
              />
              <p className="text-paragraph-xs text-foreground-muted">{t('phoneHint')}</p>
            </div>

            <div className="flex flex-col gap-y-2">
              <label
                htmlFor="send-also-email"
                className="flex cursor-pointer items-center gap-x-2.5 text-paragraph-sm text-foreground"
              >
                <Checkbox
                  id="send-also-email"
                  checked={alsoEmail}
                  onCheckedChange={(checked) => setAlsoEmail(checked === true)}
                />
                <MailIcon aria-hidden="true" className="size-4 text-foreground-muted" />
                {t('alsoEmail')}
              </label>
              {alsoEmail && (
                <Input
                  id="send-email"
                  type="email"
                  inputMode="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder={t('emailPlaceholder')}
                  aria-label={t('emailLabel')}
                />
              )}
            </div>

            <div className="flex flex-col gap-y-1.5 rounded-lg border border-border bg-sunken p-3">
              <span className="text-paragraph-xs-medium text-foreground-muted">
                {t('summaryLabel')}
              </span>
              <div className="flex items-center justify-between text-paragraph-sm">
                <span className="text-foreground-muted">{t('summaryQuote')}</span>
                <span className="tabular-nums text-foreground">
                  {formatRfqReference(detail.rfq.quote_number) ?? t('numberPending')}
                </span>
              </div>
              <div className="flex items-center justify-between text-paragraph-sm">
                <span className="text-foreground-muted">{t('summaryTotal')}</span>
                <span className="tabular-nums text-foreground">
                  {fmt.currency(detail.version?.total ?? '0')}
                </span>
              </div>
              <p className="pt-1 text-paragraph-xs text-foreground-muted">{t('summaryLink')}</p>
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={sending}
              onClick={() => handleOpenChange(false)}
            >
              {t('cancel')}
            </Button>
            <PendingButton
              type="button"
              pending={sending}
              pendingLabel={t('sending')}
              disabled={!canSend}
              onClick={handleSend}
            >
              <SendIcon className="size-4" />
              {t('send')}
            </PendingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
