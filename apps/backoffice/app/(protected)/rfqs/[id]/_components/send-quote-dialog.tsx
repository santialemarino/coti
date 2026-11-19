'use client';

import { useCallback, useRef, useState } from 'react';
import Link from 'next/link';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { MailIcon, MessageCircleIcon, SendIcon } from 'lucide-react';
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
  MultiCombobox,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { SendClientForm } from '@/app/(protected)/rfqs/[id]/_components/send-client-form';
import { PhoneInput } from '@/components/phone-input';
import { SetupNotice } from '@/components/setup-notice';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import {
  formatRfqReference,
  type QuoteDeliveryResponse,
  type QuoteSendTrackingResponse,
  type RfqDetailResponse,
  type SendClientSelection,
} from '@/lib/api/rfqs';
import { draftQuoteFollowup, sendQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

export function successfulDeliveries(deliveries: QuoteSendTrackingResponse[], versionId?: string) {
  return deliveries.filter(
    (d) =>
      d.purpose !== 'FOLLOWUP' &&
      (!versionId || d.version_id === versionId) &&
      ['SENT', 'DELIVERED', 'VIEWED'].includes(d.tracking_status),
  );
}

interface Contacts {
  phones: string[];
  emails: string[];
}
interface SendQuoteDialogProps {
  detail: RfqDetailResponse;
  branchId: string;
  onSent: () => Promise<void>;
  showTrigger?: boolean;
  branchMissesEmail?: boolean;
  resend?: boolean;
  followup?: boolean;
}

export function SendQuoteDialog({
  detail,
  branchId,
  onSent,
  showTrigger = true,
  branchMissesEmail = false,
  resend = false,
  followup = false,
}: SendQuoteDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.send');
  const tf = useTranslations('rfqs.detail.callouts.followup');
  const tChannel = useTranslations('rfqs.channels');
  const tPhone = useTranslations('common.phone');
  const message = useApiErrorMessage('rfqs.detail.send');
  const { isAdmin } = useRfqList();
  const [open, setOpen] = useState(false);
  const [clientSelection, setClientSelection] = useState<SendClientSelection | null>(null);
  const [contacts, setContacts] = useState<Contacts>({ phones: [], emails: [] });
  const [phones, setPhones] = useState<string[]>([]);
  const [emails, setEmails] = useState<string[]>([]);
  const [sendWhatsApp, setSendWhatsApp] = useState(false);
  const [alsoEmail, setAlsoEmail] = useState(false);
  const [sending, setSending] = useState(false);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [draft, setDraft] = useState('');
  const [templateConfigured, setTemplateConfigured] = useState(true);
  const [editing, setEditing] = useState(false);
  const touched = useRef(false);
  const sendAttempt = useRef<{ payload: string; key: string } | null>(null);
  const quoteId = detail.quote?.id ?? null;
  const history = successfulDeliveries(detail.deliveries, detail.version?.id);
  const blockedWhatsApp = resend && history.some((d) => d.channel === 'WHATSAPP');
  const blockedEmail = resend && history.some((d) => d.channel === 'EMAIL');
  const canSend =
    !!quoteId &&
    !loadingTemplate &&
    (clientSelection !== null || followup) &&
    (sendWhatsApp || alsoEmail) &&
    (!sendWhatsApp ||
      (!blockedWhatsApp && phones.length > 0 && phones.every((p) => isValidPhoneNumber(p)))) &&
    (!alsoEmail ||
      (!blockedEmail &&
        emails.length > 0 &&
        emails.every((e) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(e)))) &&
    (!followup || draft.trim().length > 0);

  const adoptContacts = useCallback(
    (incoming: Contacts) => {
      const successful = successfulDeliveries(detail.deliveries, detail.version?.id);
      const previous = successfulDestinations(successful);
      const fallbackPhone = detail.inbound_whatsapp_phone || detail.publicRequest?.phone;
      const fallbackEmail = detail.inbound_email || detail.publicRequest?.email;
      const hasContacts = incoming.phones.length > 0 || incoming.emails.length > 0;
      const next = hasContacts
        ? incoming
        : {
            phones: fallbackPhone ? [fallbackPhone] : [],
            emails: fallbackEmail ? [fallbackEmail] : [],
          };
      setContacts(next);
      if (touched.current) return;
      const blockedPhone = resend && successful.some((d) => d.channel === 'WHATSAPP');
      const blockedMail = resend && successful.some((d) => d.channel === 'EMAIL');
      const usePrevious = followup && (previous.phones.length > 0 || previous.emails.length > 0);
      const selectedPhones = usePrevious ? previous.phones : next.phones.slice(0, 1);
      const selectedEmails = usePrevious ? previous.emails : next.emails.slice(0, 1);
      setPhones(selectedPhones);
      setEmails(selectedEmails);
      const whatsapp = !blockedPhone && selectedPhones.length > 0;
      setSendWhatsApp(whatsapp);
      setAlsoEmail(!blockedMail && selectedEmails.length > 0 && (usePrevious || !whatsapp));
    },
    [
      detail.deliveries,
      detail.version?.id,
      detail.inbound_whatsapp_phone,
      detail.inbound_email,
      detail.publicRequest,
      followup,
      resend,
    ],
  );

  function handleOpenChange(next: boolean) {
    if (sending) return;
    if (next) {
      touched.current = false;
      sendAttempt.current = null;
      setClientSelection(null);
      setDraft('');
      setEditing(false);
      setTemplateConfigured(true);
      adoptContacts({ phones: [], emails: [] });
      if (followup && quoteId) {
        setLoadingTemplate(true);
        void draftQuoteFollowup(quoteId, branchId)
          .then((result) => {
            setDraft(result.message);
            setTemplateConfigured(result.templateConfigured === true);
          })
          .catch((error) => {
            toast.error(message(errorCodeOf(error)));
            setOpen(false);
          })
          .finally(() => setLoadingTemplate(false));
      }
    }
    setOpen(next);
  }

  async function handleSend() {
    if (!quoteId || !canSend) return;
    setSending(true);
    try {
      const body = {
        recipientPhones: sendWhatsApp ? phones : [],
        recipientEmails: alsoEmail ? emails.map((e) => e.trim()) : [],
        clientAssociation: clientSelection ?? undefined,
        otherChannelsOnly: resend,
        ...(followup ? { followupMessage: draft.trim() } : {}),
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
  function reportOutcome(deliveries: QuoteDeliveryResponse[]) {
    const external = deliveries.filter((d) => d.channel !== 'WEBAPP');
    const failed = external.filter((d) => d.tracking_status === 'FAILED');
    if (!failed.length) {
      toast.success(followup ? tf('sent') : t('toast.sent'));
      return;
    }
    if (failed.length === external.length) {
      toast.error(t('toast.failed'));
      return;
    }
    toast.warning(
      t('toast.partial', {
        channels: fmt.list([...new Set(failed.map((d) => tChannel(d.channel.toLowerCase())))]),
      }),
    );
  }
  return (
    <>
      {showTrigger ? (
        <Button
          type="button"
          variant={resend || followup ? 'outline' : 'default'}
          size={resend || followup ? 'sm' : 'default'}
          onClick={() => handleOpenChange(true)}
        >
          <SendIcon className="size-4" />
          {followup ? tf('send') : t(resend ? 'resend' : 'button')}
        </Button>
      ) : null}
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{followup ? tf('send') : t('title')}</DialogTitle>
            <DialogDescription className="sr-only">
              {followup ? tf('description') : t('description')}
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            {open && quoteId ? (
              <SendClientForm
                quoteId={quoteId}
                branchId={branchId}
                disabled={sending}
                locked={
                  followup ||
                  detail.rfq.channel.toLowerCase() === 'webapp' ||
                  detail.quote?.current_status === 'SENT'
                }
                hideTags={followup}
                onChange={setClientSelection}
                onContactsChange={adoptContacts}
              />
            ) : null}
            {followup ? (
              <div className="flex flex-col gap-3">
                {editing ? (
                  <Textarea
                    aria-label={tf('draft.label')}
                    maxLength={2000}
                    value={draft}
                    disabled={sending || loadingTemplate}
                    onChange={(e) => setDraft(e.target.value)}
                  />
                ) : (
                  <p className="whitespace-pre-wrap text-paragraph-sm">
                    {loadingTemplate ? tf('loading') : draft}
                  </p>
                )}
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={sending || loadingTemplate}
                    onClick={() => setEditing((v) => !v)}
                  >
                    {tf(editing ? 'doneEditing' : 'edit')}
                  </Button>
                  <CopyButton
                    value={draft}
                    labels={{ copy: tf('draft.copy'), copied: tf('draft.copied') }}
                    onCopyError={() => toast.error(tf('draft.copyFailed'))}
                  />
                  {!templateConfigured && isAdmin ? (
                    <Button asChild variant="outline" size="sm">
                      <Link href={ROUTES.accountSettings + '#followup-template'}>
                        {tf('configure')}
                      </Link>
                    </Button>
                  ) : null}
                </div>
              </div>
            ) : null}
            <fieldset disabled={sending} className="flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label className="flex items-center gap-2">
                  <Checkbox
                    checked={sendWhatsApp}
                    disabled={blockedWhatsApp}
                    onCheckedChange={(v) => {
                      touched.current = true;
                      setSendWhatsApp(v === true);
                    }}
                  />
                  <MessageCircleIcon aria-hidden="true" className="size-4" />
                  {t('sendWhatsApp')}
                  {blockedWhatsApp ? (
                    <span className="text-paragraph-xs text-foreground-muted">
                      {t('alreadySent')}
                    </span>
                  ) : null}
                </Label>
                <Label htmlFor="send-phone">{t('phoneLabel')}</Label>
                {contacts.phones.length > 1 ? (
                  <MultiCombobox
                    id="send-phone"
                    values={phones}
                    onValuesChange={(v) => {
                      touched.current = true;
                      setPhones(v);
                    }}
                    options={contacts.phones.map((value) => ({ value, label: value }))}
                    placeholder={t('phoneLabel')}
                    summaryLabel={(count) => t('selectedRecipients', { count })}
                    clearLabel={t('clearRecipients')}
                    aria-label={t('phoneLabel')}
                    disabled={blockedWhatsApp}
                  />
                ) : (
                  <PhoneInput
                    id="send-phone"
                    value={phones[0] ?? ''}
                    onChange={(v) => {
                      touched.current = true;
                      setPhones(v ? [v] : []);
                    }}
                    aria-label={t('phoneLabel')}
                    disabled={blockedWhatsApp}
                    countryLabel={tPhone('country')}
                    countrySearchPlaceholder={tPhone('countrySearch')}
                    countryEmptyLabel={tPhone('countryEmpty')}
                    landlineNotice={{
                      message: tPhone('landline'),
                      action: (mobile) => tPhone('useMobile', { number: mobile }),
                    }}
                  />
                )}
              </div>
              <div className="flex flex-col gap-2">
                <Label className="flex items-center gap-2">
                  <Checkbox
                    checked={alsoEmail}
                    disabled={blockedEmail}
                    onCheckedChange={(v) => {
                      touched.current = true;
                      setAlsoEmail(v === true);
                    }}
                  />
                  <MailIcon aria-hidden="true" className="size-4" />
                  {t('alsoEmail')}
                  {blockedEmail ? (
                    <span className="text-paragraph-xs text-foreground-muted">
                      {t('alreadySent')}
                    </span>
                  ) : null}
                </Label>
                {contacts.emails.length > 1 ? (
                  <MultiCombobox
                    values={emails}
                    onValuesChange={(v) => {
                      touched.current = true;
                      setEmails(v);
                    }}
                    options={contacts.emails.map((value) => ({ value, label: value }))}
                    placeholder={t('emailLabel')}
                    summaryLabel={(count) => t('selectedRecipients', { count })}
                    clearLabel={t('clearRecipients')}
                    aria-label={t('emailLabel')}
                    disabled={blockedEmail}
                  />
                ) : (
                  <Input
                    type="email"
                    value={emails[0] ?? ''}
                    onChange={(e) => {
                      touched.current = true;
                      setEmails(e.target.value ? [e.target.value] : []);
                    }}
                    aria-label={t('emailLabel')}
                    placeholder={t('emailPlaceholder')}
                    disabled={blockedEmail}
                  />
                )}
                {alsoEmail && branchMissesEmail ? (
                  <SetupNotice issue="BRANCH_EMAIL" isAdmin={isAdmin} />
                ) : null}
              </div>
            </fieldset>
            {!followup ? (
              <div className="flex items-center justify-between p-3 bg-sunken rounded-lg text-paragraph-sm">
                <span>{formatRfqReference(detail.rfq.quote_number) ?? t('numberPending')}</span>
                <span>{fmt.currency(detail.version?.total ?? '0')}</span>
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" disabled={sending} onClick={() => handleOpenChange(false)}>
              {t('cancel')}
            </Button>
            <PendingButton
              pending={sending}
              pendingLabel={t('sending')}
              disabled={!canSend}
              onClick={handleSend}
            >
              <SendIcon className="size-4" />
              {followup ? tf('confirmSend') : t('send')}
            </PendingButton>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
function successfulDestinations(deliveries: QuoteSendTrackingResponse[]): Contacts {
  const latest = [...deliveries].sort((a, b) => a.created_at.localeCompare(b.created_at))[0];
  const operation = deliveries.filter((d) => d.operation_id === latest?.operation_id);
  return {
    phones: [
      ...new Set(operation.filter((d) => d.channel === 'WHATSAPP').map((d) => d.destination)),
    ],
    emails: [...new Set(operation.filter((d) => d.channel === 'EMAIL').map((d) => d.destination))],
  };
}
