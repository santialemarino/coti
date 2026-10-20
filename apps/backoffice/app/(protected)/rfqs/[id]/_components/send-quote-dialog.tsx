'use client';

import { useState } from 'react';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
import {
  ExternalLinkIcon,
  FileSymlinkIcon,
  MailIcon,
  MessageCircleIcon,
  SendIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Checkbox,
  CopyButton,
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
  type QuoteSendResponse,
  type QuoteSendTrackingResponse,
  type RfqDetailResponse,
} from '@/lib/api/rfqs';
import { sendQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

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
  /*
   * The send moves the quote out of QUOTED, which is what offers the trigger. The dialog stays
   * mounted without it, so the delivered links remain on screen after the card behind has moved on.
   */
  showTrigger?: boolean;
  /* Whether the order's branch has no mailbox for a reply to the email copy to reach. */
  branchMissesEmail?: boolean;
}

// Deliver an approved quote through the channels selected by the seller.
export function SendQuoteDialog({
  detail,
  branchId,
  onSent,
  showTrigger = true,
  branchMissesEmail = false,
}: SendQuoteDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.send');
  const tChannel = useTranslations('rfqs.channels');
  const tPhone = useTranslations('common.phone');
  const message = useApiErrorMessage('rfqs.detail.send');
  const { isAdmin } = useRfqList();
  const [open, setOpen] = useState(false);
  const [phone, setPhone] = useState('');
  const [sendWhatsApp, setSendWhatsApp] = useState(true);
  const [alsoEmail, setAlsoEmail] = useState(false);
  const [email, setEmail] = useState('');
  const [sending, setSending] = useState(false);
  const [result, setResult] = useState<QuoteSendResponse | null>(null);

  const quoteId = detail.quote?.id ?? null;
  const phoneValid = !sendWhatsApp || isValidPhoneNumber(phone);
  const emailValid = !alsoEmail || /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
  const canSend = !!quoteId && (sendWhatsApp || alsoEmail) && phoneValid && emailValid;

  // The sent URLs are the recovery surface for a message that never arrived, so the dialog holds
  // them until it closes rather than vanishing the moment the send returns.
  const successfulDeliveries = result?.deliveries.filter(
    (delivery) => delivery.tracking_status !== 'FAILED',
  );

  // Reset on the way in: resetting on the way out would swap the success view for the form mid-exit.
  function handleOpenChange(next: boolean) {
    if (sending) return;
    if (next) {
      const previous = previousDestinations(
        detail.deliveries,
        detail.inbound_whatsapp_phone,
        detail.inbound_email,
        detail.rfq.channel,
      );
      setResult(null);
      setPhone(previous.phone);
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
      const result = await sendQuote(quoteId, branchId, {
        ...(sendWhatsApp ? { recipient_phone: phone } : {}),
        email_delivery: alsoEmail ? { address: email.trim() } : null,
      });
      reportOutcome(result.deliveries);
      setResult(result);
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
        <Button type="button" onClick={() => handleOpenChange(true)}>
          <SendIcon className="size-4" />
          {t('button')}
        </Button>
      ) : null}

      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{result ? t('successTitle') : t('title')}</DialogTitle>
            <DialogDescription>
              {result ? t('successDescription') : t('description')}
            </DialogDescription>
          </DialogHeader>

          {result && successfulDeliveries ? (
            /*
             * The backend already put the link in each working channel's message, so this is a
             * recovery surface, not the delivery itself: the seller can copy or reopen a link when
             * the customer lost or never got the message. A channel that failed carries no link and
             * was already named by the toast that reported the partial outcome.
             */
            <div className="flex flex-col gap-y-3">
              {successfulDeliveries.map((delivery) => (
                <div
                  key={delivery.id}
                  className="flex flex-col p-3 gap-y-3 border border-border bg-sunken rounded-lg"
                >
                  {delivery.channel.toLowerCase() !== 'webapp' && (
                    <div className="flex flex-col gap-y-1">
                      <span className="text-paragraph-xs-medium text-foreground-muted">
                        {tChannel(delivery.channel.toLowerCase())}
                      </span>
                      <span className="text-paragraph-sm text-foreground">
                        {delivery.destination}
                      </span>
                      <span
                        aria-label={t('linkLabel')}
                        className="text-paragraph-xs text-foreground-muted break-all"
                      >
                        {delivery.public_url}
                      </span>
                    </div>
                  )}
                  <div className="flex gap-x-2">
                    <CopyButton
                      value={delivery.public_url}
                      labels={{ copy: t('copyLink'), copied: t('copiedLink') }}
                      onCopyError={() => toast.error(t('copyFailed'))}
                    />
                    <Button asChild variant="outline" size="sm">
                      <a href={delivery.public_url} target="_blank" rel="noopener noreferrer">
                        {delivery.channel.toLowerCase() === 'webapp' ? (
                          <FileSymlinkIcon aria-hidden="true" className="size-3.5" />
                        ) : (
                          <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
                        )}
                        {t(
                          delivery.channel.toLowerCase() === 'webapp'
                            ? 'visualizeOnline'
                            : 'openQuote',
                        )}
                      </a>
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex flex-col gap-y-4">
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
                <p className="pt-1 text-paragraph-xs text-foreground-muted">{t('summaryLink')}</p>
              </div>
            </div>
          )}

          <DialogFooter>
            {result ? (
              <>
                <Button type="button" variant="outline" onClick={() => setResult(null)}>
                  {t('resend')}
                </Button>
                <Button type="button" onClick={() => handleOpenChange(false)}>
                  {t('done')}
                </Button>
              </>
            ) : (
              <>
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
              </>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
