'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { MailIcon, MessageCircleIcon, RefreshCwIcon, SendIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
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
  type QuoteDeliveryMode,
  type QuoteDeliveryPreset,
  type QuoteDeliveryResponse,
  type RfqDetailResponse,
  type SendClientSelection,
} from '@/lib/api/rfqs';
import { draftQuoteFollowup, fetchDeliveryPreset, sendQuote } from '@/lib/api/rfqs-client';
import { useFormatters } from '@/lib/i18n/formatters';

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export interface Contacts {
  phones: string[];
  emails: string[];
}

export interface DeliverySelection extends Contacts {
  whatsapp: boolean;
  email: boolean;
}

/*
 * The one rule every send dialog starts from. A preset (this quote's earlier destinations, or the
 * client's last quote) selects exactly what it names. Without one, every known contact is prefilled
 * and only WhatsApp is checked when it has a number, else email when it has an address; a medium
 * the cycle already reached is never checked in «otro medio», and nothing is checked with no data.
 */
export function initialSelection(
  contacts: Contacts,
  preset: QuoteDeliveryPreset | null,
  mode: QuoteDeliveryMode,
): DeliverySelection {
  if (preset && preset.source !== 'NONE' && mode !== 'OTHER_MEDIUM') {
    return {
      phones: preset.phones,
      emails: preset.emails,
      whatsapp: preset.phones.length > 0,
      email: preset.emails.length > 0,
    };
  }
  const usedWhatsApp = mode === 'OTHER_MEDIUM' && (preset?.usedPhones.length ?? 0) > 0;
  const usedEmail = mode === 'OTHER_MEDIUM' && (preset?.usedEmails.length ?? 0) > 0;
  const phones = contacts.phones.slice(0, 1);
  const emails = contacts.emails.slice(0, 1);
  const whatsapp = !usedWhatsApp && phones.length > 0;
  return { phones, emails, whatsapp, email: !whatsapp && !usedEmail && emails.length > 0 };
}

interface SendQuoteDialogProps {
  detail: RfqDetailResponse;
  branchId: string;
  onSent: () => Promise<void>;
  mode?: QuoteDeliveryMode;
  branchMissesEmail?: boolean;
}

export function SendQuoteDialog({
  detail,
  branchId,
  onSent,
  mode = 'FIRST',
  branchMissesEmail = false,
}: SendQuoteDialogProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.detail.send');
  const tf = useTranslations('rfqs.detail.callouts.followup');
  const tChannel = useTranslations('rfqs.channels');
  const message = useApiErrorMessage('rfqs.detail.send');
  const { isAdmin } = useRfqList();
  const [open, setOpen] = useState(false);
  const [clientSelection, setClientSelection] = useState<SendClientSelection | null>(null);
  const [contacts, setContacts] = useState<Contacts>({ phones: [], emails: [] });
  const [preset, setPreset] = useState<QuoteDeliveryPreset | null>(null);
  const [selection, setSelection] = useState<DeliverySelection>({
    phones: [],
    emails: [],
    whatsapp: false,
    email: false,
  });
  const [sending, setSending] = useState(false);
  const [loadingTemplate, setLoadingTemplate] = useState(false);
  const [draft, setDraft] = useState('');
  const [templateConfigured, setTemplateConfigured] = useState(true);
  const [editing, setEditing] = useState(false);
  const touched = useRef(false);
  const presetRequest = useRef(0);
  const sendAttempt = useRef<{ payload: string; key: string } | null>(null);
  const followup = mode === 'FOLLOWUP';
  const quoteId = detail.quote?.id ?? null;
  const usedPhones = mode === 'OTHER_MEDIUM' ? (preset?.usedPhones ?? []) : [];
  const usedEmails = mode === 'OTHER_MEDIUM' ? (preset?.usedEmails ?? []) : [];
  const blockedWhatsApp = usedPhones.length > 0;
  const blockedEmail = usedEmails.length > 0;
  const allUsed = mode === 'OTHER_MEDIUM' && blockedWhatsApp && blockedEmail;
  const phoneOptions = [...new Set([...selection.phones, ...contacts.phones])];
  const emailOptions = [...new Set([...selection.emails, ...contacts.emails])];
  const canSend =
    !!quoteId &&
    preset !== null &&
    !loadingTemplate &&
    (clientSelection !== null || mode !== 'FIRST') &&
    (selection.whatsapp || selection.email) &&
    (!selection.whatsapp ||
      (!blockedWhatsApp &&
        selection.phones.length > 0 &&
        selection.phones.every((p) => isValidPhoneNumber(p)))) &&
    (!selection.email ||
      (!blockedEmail &&
        selection.emails.length > 0 &&
        selection.emails.every((e) => EMAIL_PATTERN.test(e)))) &&
    (!followup || draft.trim().length > 0);

  const adoptContacts = useCallback(
    (incoming: Contacts) => {
      const fallbackPhone = detail.inbound_whatsapp_phone || detail.publicRequest?.phone;
      const fallbackEmail = detail.inbound_email || detail.publicRequest?.email;
      const hasContacts = incoming.phones.length > 0 || incoming.emails.length > 0;
      setContacts(
        hasContacts
          ? incoming
          : {
              phones: fallbackPhone ? [fallbackPhone] : [],
              emails: fallbackEmail ? [fallbackEmail] : [],
            },
      );
    },
    [detail.inbound_whatsapp_phone, detail.inbound_email, detail.publicRequest],
  );

  // The client chosen in the dialog decides a first send's preset; later modes keep the quote's.
  const chosenClientId = mode === 'FIRST' ? (clientSelection?.clientId ?? null) : null;
  useEffect(() => {
    if (!open || !quoteId) return;
    const request = ++presetRequest.current;
    setPreset(null);
    fetchDeliveryPreset(quoteId, branchId, mode, chosenClientId)
      .then((next) => {
        if (request === presetRequest.current) setPreset(next);
      })
      .catch((error) => {
        if (request !== presetRequest.current) return;
        toast.error(message(errorCodeOf(error)));
        setOpen(false);
      });
  }, [open, quoteId, branchId, mode, chosenClientId, message]);

  useEffect(() => {
    if (!preset || touched.current) return;
    setSelection(initialSelection(contacts, preset, mode));
  }, [contacts, preset, mode]);

  function update(next: Partial<DeliverySelection>) {
    touched.current = true;
    setSelection((current) => ({ ...current, ...next }));
  }

  function handleOpenChange(next: boolean) {
    if (sending) return;
    if (next) {
      touched.current = false;
      sendAttempt.current = null;
      setClientSelection(null);
      setPreset(null);
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
        mode,
        recipientPhones: selection.whatsapp ? selection.phones : [],
        recipientEmails: selection.email ? selection.emails.map((e) => e.trim()) : [],
        clientAssociation: mode === 'FIRST' ? (clientSelection ?? undefined) : undefined,
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

  const title = followup ? tf('send') : t(`modes.${mode}.title`);
  const trigger = followup ? tf('send') : t(`modes.${mode}.trigger`);
  const TriggerIcon = mode === 'RESEND' ? RefreshCwIcon : SendIcon;

  return (
    <>
      <Button
        type="button"
        variant={mode === 'FIRST' ? 'default' : 'outline'}
        size={mode === 'FIRST' ? 'default' : 'sm'}
        onClick={() => handleOpenChange(true)}
      >
        <TriggerIcon className="size-4" />
        {trigger}
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="sm:max-w-lg" closeOnClickOutside={!sending}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className="sr-only">
              {followup ? tf('description') : t('description')}
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-y-4">
            {open && quoteId ? (
              <SendClientForm
                quoteId={quoteId}
                branchId={branchId}
                disabled={sending}
                locked={
                  mode !== 'FIRST' ||
                  detail.rfq.channel.toLowerCase() === 'webapp' ||
                  detail.quote?.current_status === 'SENT'
                }
                hideTags={mode !== 'FIRST'}
                onChange={setClientSelection}
                onContactsChange={adoptContacts}
              />
            ) : null}
            {followup ? (
              <div className="flex flex-col gap-y-3">
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
            {allUsed ? <Callout tone="info">{t('allUsed')}</Callout> : null}
            <fieldset disabled={sending || preset === null} className="flex flex-col gap-y-4">
              <DestinationField
                kind="whatsapp"
                checked={selection.whatsapp}
                values={selection.phones}
                options={phoneOptions}
                used={usedPhones}
                onCheckedChange={(whatsapp) => update({ whatsapp })}
                onValuesChange={(phones) => update({ phones })}
              />
              <DestinationField
                kind="email"
                checked={selection.email}
                values={selection.emails}
                options={emailOptions}
                used={usedEmails}
                onCheckedChange={(email) => update({ email })}
                onValuesChange={(emails) => update({ emails })}
              />
              {selection.email && branchMissesEmail ? (
                <SetupNotice issue="BRANCH_EMAIL" isAdmin={isAdmin} />
              ) : null}
              {preset && preset.source !== 'NONE' && mode !== 'OTHER_MEDIUM' ? (
                <p className="text-paragraph-xs text-foreground-muted">
                  {t(`presetSource.${preset.source}`)}
                </p>
              ) : null}
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

interface DestinationFieldProps {
  kind: 'whatsapp' | 'email';
  checked: boolean;
  values: string[];
  options: string[];
  used: string[];
  onCheckedChange: (checked: boolean) => void;
  onValuesChange: (values: string[]) => void;
}

/*
 * One medium: its checkbox, then a single field while there is at most one known value, or a
 * multi-select of the known ones. Either way a new number or address can be added, and with several
 * known a hint says only the selected ones receive it. A medium this cycle already reached shows
 * where it went instead of an editable field.
 */
function DestinationField({
  kind,
  checked,
  values,
  options,
  used,
  onCheckedChange,
  onValuesChange,
}: DestinationFieldProps) {
  const t = useTranslations('rfqs.detail.send');
  const tPhone = useTranslations('common.phone');
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState('');
  const whatsapp = kind === 'whatsapp';
  const id = `send-${kind}`;
  const label = t(whatsapp ? 'phoneLabel' : 'emailLabel');
  const valid = (value: string) =>
    whatsapp ? isValidPhoneNumber(value) : EMAIL_PATTERN.test(value.trim());
  const multiple = options.length > 1;
  const Icon = whatsapp ? MessageCircleIcon : MailIcon;
  const landlineNotice = {
    message: tPhone('landline'),
    action: (mobile: string) => tPhone('useMobile', { number: mobile }),
  };

  function add() {
    const value = draft.trim();
    if (!valid(value)) return;
    onValuesChange([...new Set([...values, value])]);
    onCheckedChange(true);
    setDraft('');
    setAdding(false);
  }

  return (
    <div className="flex flex-col gap-y-2">
      <Label className="flex items-center gap-x-2">
        <Checkbox
          checked={checked && used.length === 0}
          disabled={used.length > 0}
          onCheckedChange={(v) => onCheckedChange(v === true)}
        />
        <Icon aria-hidden="true" className="size-4" />
        {t(whatsapp ? 'sendWhatsApp' : 'alsoEmail')}
      </Label>
      {used.length > 0 ? (
        <p className="text-paragraph-xs text-foreground-muted">
          {t('usedOn', { destinations: used.join(', ') })}
        </p>
      ) : (
        <>
          {multiple ? (
            <MultiCombobox
              id={id}
              values={values}
              onValuesChange={onValuesChange}
              options={options.map((value) => ({ value, label: value }))}
              placeholder={label}
              summaryLabel={(count) => t('selectedRecipients', { count })}
              clearLabel={t('clearRecipients')}
              aria-label={label}
            />
          ) : whatsapp ? (
            <PhoneInput
              id={id}
              value={values[0] ?? ''}
              onChange={(v) => onValuesChange(v ? [v] : [])}
              aria-label={label}
              countryLabel={tPhone('country')}
              countrySearchPlaceholder={tPhone('countrySearch')}
              countryEmptyLabel={tPhone('countryEmpty')}
              landlineNotice={landlineNotice}
            />
          ) : (
            <Input
              id={id}
              type="email"
              value={values[0] ?? ''}
              onChange={(e) => onValuesChange(e.target.value ? [e.target.value] : [])}
              aria-label={label}
              placeholder={t('emailPlaceholder')}
            />
          )}
          {multiple ? (
            <p className="text-paragraph-xs text-foreground-muted">
              {t('onlySelected', { selected: values.length, total: options.length })}
            </p>
          ) : null}
          {adding ? (
            <div className="flex items-start gap-x-2">
              <div className="min-w-0 flex-1">
                {whatsapp ? (
                  <PhoneInput
                    value={draft}
                    onChange={setDraft}
                    aria-label={t('addPhone')}
                    countryLabel={tPhone('country')}
                    countrySearchPlaceholder={tPhone('countrySearch')}
                    countryEmptyLabel={tPhone('countryEmpty')}
                    landlineNotice={landlineNotice}
                  />
                ) : (
                  <Input
                    type="email"
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    aria-label={t('addEmail')}
                    placeholder={t('emailPlaceholder')}
                  />
                )}
              </div>
              <Button variant="outline" disabled={!valid(draft)} onClick={add}>
                {t('add')}
              </Button>
            </div>
          ) : values.length > 0 && values.every(valid) ? (
            <Button
              variant="ghost"
              size="sm"
              className="self-start"
              onClick={() => setAdding(true)}
            >
              {t(whatsapp ? 'addPhone' : 'addEmail')}
            </Button>
          ) : null}
        </>
      )}
    </div>
  );
}
