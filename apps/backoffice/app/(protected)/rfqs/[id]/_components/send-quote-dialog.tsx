'use client';

import { useRef, useState } from 'react';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
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
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { PhoneInput } from '@/components/phone-input';
import { SetupNotice } from '@/components/setup-notice';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import {
  formatRfqReference,
  type QuoteDeliveryResponse,
  type QuoteSendTrackingResponse,
  type RfqDetailResponse,
  type SendClientSelection,
} from '@/lib/api/rfqs';
import { sendQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';
import { SendClientForm } from './send-client-form';

interface Destinations {
  phone: string;
  email: string | null;
  whatsapp: boolean;
}

// Each send's rows share an operation ID even when a version is sent more than once.
export function previousDestinations(
  deliveries: QuoteSendTrackingResponse[],
  inboundPhone: string | null,
  inboundEmail: string | null,
  inboundChannel: string,
): Destinations {
  const latest = [...deliveries].sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  if (!latest) {
    return {
      phone: inboundPhone ?? '',
      email: inboundChannel === 'email' ? (inboundEmail ?? '') : null,
      whatsapp: inboundChannel !== 'email',
    };
  }
  const sameSend = deliveries.filter((delivery) => delivery.operation_id === latest.operation_id);
  const whatsapp = sameSend.find((delivery) => delivery.channel === 'WHATSAPP');
  const email = sameSend.find((delivery) => delivery.channel === 'EMAIL');
  return {
    phone: whatsapp?.destination ?? inboundPhone ?? '',
    email: email?.destination ?? null,
    whatsapp: !!whatsapp,
  };
}

interface SendQuoteDialogProps {
  detail: RfqDetailResponse;
  branchId: string;
  onSent: () => Promise<void>;
  showTrigger?: boolean;
  /* Whether the order's branch has no mailbox for a reply to the email copy to reach. */
  branchMissesEmail?: boolean;
  resend?: boolean;
}

// Deliver an approved quote through the channels selected by the seller.
export function SendQuoteDialog({
  detail,
  branchId,
  onSent,
  showTrigger = true,
  branchMissesEmail = false,
  resend = false,
}: SendQuoteDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.send');
  const tChannel = useTranslations('rfqs.channels');
  const tPhone = useTranslations('common.phone');
  const message = useApiErrorMessage('rfqs.detail.send');
  const { isAdmin } = useRfqList();
  const [open, setOpen] = useState(false);
  const [clientSelection, setClientSelection] = useState<SendClientSelection | null>(null);
  const [phone, setPhone] = useState('');
  const [sendWhatsApp, setSendWhatsApp] = useState(true);
  const [alsoEmail, setAlsoEmail] = useState(false);
  const [email, setEmail] = useState('');
  const [sending, setSending] = useState(false);
  const sendAttempt = useRef<{ payload: string; key: string } | null>(null);

  const quoteId = detail.quote?.id ?? null;
  const phoneValid = !sendWhatsApp || isValidPhoneNumber(phone);
  const emailValid = !alsoEmail || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
  const canSend =
    !!quoteId &&
    (sendWhatsApp || alsoEmail) &&
    phoneValid &&
    emailValid &&
    clientSelection !== null;

  // Reset on open so closing keeps the reviewed form intact through its exit.
  function handleOpenChange(next: boolean) {
    if (sending) return;
    if (next) {
      sendAttempt.current = null;
      const previous = previousDestinations(
        detail.deliveries,
        detail.inbound_whatsapp_phone,
        detail.inbound_email,
        detail.rfq.channel,
      );
      if (detail.deliveries.length === 0 && detail.publicRequest) {
        const contact = detail.publicRequest;
        previous.phone = contact.phone || previous.phone;
        previous.whatsapp = !!previous.phone;
        previous.email = contact.email || null;
      }
      setClientSelection(null);
      setPhone(previous.phone || detail.publicRequest?.phone || '');
      setSendWhatsApp(previous.whatsapp);
      setAlsoEmail(previous.email !== null);
      setEmail(previous.email ?? '');
    }
    setOpen(next);
  }

  async function handleSend() {
    if (!quoteId || !canSend) return;
    setSending(true);
    try {
      const body = {
        ...(sendWhatsApp ? { recipient_phone: phone } : {}),
        clientAssociation: clientSelection ?? undefined,
        email_delivery: alsoEmail ? { address: email.trim() } : null,
      };
      const payload = JSON.stringify(body);
      const key =
        sendAttempt.current?.payload === payload ? sendAttempt.current.key : crypto.randomUUID();
      sendAttempt.current = { payload, key };
      const result = await sendQuote(quoteId, branchId, body, key);
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
      t('toast.partial', {
        channels: fmt.list(failed.map((delivery) => tChannel(delivery.channel.toLowerCase()))),
      }),
    );
  }

  return (
    <>
      {showTrigger ? (
        <Button
          type="button"
          variant={resend ? 'outline' : 'default'}
          size={resend ? 'sm' : 'default'}
          onClick={() => handleOpenChange(true)}
        >
          <SendIcon className="size-4" />
          {t(resend ? 'resend' : 'button')}
        </Button>
      ) : null}

      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription className="sr-only">{t('description')}</DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-y-4">
            {open && quoteId ? (
              <SendClientForm
                quoteId={quoteId}
                branchId={branchId}
                disabled={sending}
                locked={
                  detail.rfq.channel.toLowerCase() === 'webapp' ||
                  detail.quote?.current_status === 'SENT' ||
                  detail.deliveries.some((delivery) =>
                    ['SENT', 'DELIVERED', 'VIEWED'].includes(delivery.tracking_status),
                  )
                }
                onChange={setClientSelection}
              />
            ) : null}
            <div className="flex flex-col gap-y-2">
              <label
                htmlFor="send-whatsapp"
                className="flex cursor-pointer items-center gap-x-2.5 text-paragraph-sm text-foreground"
              >
                <Checkbox
                  id="send-whatsapp"
                  checked={sendWhatsApp}
                  onCheckedChange={(checked) => setSendWhatsApp(checked === true)}
                />
                <MessageCircleIcon aria-hidden="true" className="size-4 text-foreground-muted" />
                {t('sendWhatsApp')}
              </label>
              {sendWhatsApp ? (
                <div className="flex flex-col gap-y-1.5">
                  <Label htmlFor="send-phone" required>
                    {t('phoneLabel')}
                  </Label>
                  <PhoneInput
                    id="send-phone"
                    value={phone}
                    onChange={setPhone}
                    placeholder={t('phonePlaceholder')}
                    countryLabel={tPhone('country')}
                    countrySearchPlaceholder={tPhone('countrySearch')}
                    countryEmptyLabel={tPhone('countryEmpty')}
                    landlineNotice={{
                      message: tPhone('landline'),
                      action: (mobile) => tPhone('useMobile', { number: mobile }),
                    }}
                    aria-describedby="send-phone-hint"
                    autoFocus
                  />
                  <p id="send-phone-hint" className="text-paragraph-xs text-foreground-muted">
                    {t('phoneHint')}
                  </p>
                </div>
              ) : null}
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
                  autoFocus={!sendWhatsApp}
                />
              )}
              {/* Said where it bites: the copy goes out, and its reply has nowhere to land. */}
              {alsoEmail && branchMissesEmail ? (
                <SetupNotice issue="BRANCH_EMAIL" isAdmin={isAdmin} />
              ) : null}
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
