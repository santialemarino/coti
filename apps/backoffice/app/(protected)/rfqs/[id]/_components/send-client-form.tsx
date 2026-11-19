'use client';

import { useEffect, useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';
import { z } from 'zod';

import {
  Button,
  Checkbox,
  Combobox,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  Label,
  Spinner,
} from '@repo/ui/components';
import { PhoneInput } from '@/components/phone-input';
import type { ClientMatch, QuoteClientAssociation } from '@/lib/api/client-profiles';
import { getClientDirectory, getQuoteClientAssociation } from '@/lib/api/clients-client';
import type { SendClientSelection } from '@/lib/api/rfqs';

interface SendClientFormProps {
  quoteId: string;
  branchId: string;
  disabled: boolean;
  locked?: boolean;
  hideTags?: boolean;
  onContactsChange?: (contacts: { phones: string[]; emails: string[] }) => void;
  onChange: (selection: SendClientSelection | null) => void;
}

export function SendClientForm({
  quoteId,
  branchId,
  disabled,
  locked = false,
  hideTags = false,
  onChange,
  onContactsChange,
}: SendClientFormProps) {
  const t = useTranslations('rfqs.detail.send');
  const phoneCopy = useTranslations('common.phone');
  const [newOpen, setNewOpen] = useState(false);
  const [newConfirmed, setNewConfirmed] = useState(false);
  const [context, setContext] = useState<QuoteClientAssociation | null>(null);
  const [directory, setDirectory] = useState<ClientMatch[]>([]);
  const [failed, setFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const schema = useMemo(
    () =>
      z
        .object({
          mode: z.enum(['existing', 'new']),
          clientId: z.string(),
          taxId: z.string(),
          name: z.string().max(255),
          email: z.string().trim(),
          phone: z.string(),
          tagIds: z.array(z.string()),
        })
        .superRefine((values, ctx) => {
          if (values.mode === 'new' && !values.email && !values.phone)
            ctx.addIssue({ code: 'custom', path: ['email'], message: t('clientContactRequired') });
          if (values.mode === 'new' && !values.name.trim())
            ctx.addIssue({ code: 'custom', path: ['name'], message: t('clientNameRequired') });
          if (
            values.mode === 'new' &&
            values.email &&
            !z.email().max(255).safeParse(values.email).success
          )
            ctx.addIssue({ code: 'custom', path: ['email'], message: t('clientEmailInvalid') });
          if (values.mode === 'new' && values.phone && !isValidPhoneNumber(values.phone))
            ctx.addIssue({ code: 'custom', path: ['phone'], message: t('clientPhoneInvalid') });
          if (values.mode === 'existing' && !values.clientId)
            ctx.addIssue({ code: 'custom', path: ['clientId'], message: t('clientChoose') });
        }),
    [t],
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    mode: 'onChange',
    defaultValues: {
      mode: 'existing',
      clientId: '',
      taxId: '',
      name: '',
      email: '',
      phone: '',
      tagIds: [],
    },
  });
  const values = useWatch({ control: form.control });
  const selected = directory.find((item) => item.client.id === values.clientId);
  const fixedClient = locked && (context?.currentClient != null || selected != null);
  const checked = schema.safeParse(values);
  const selection: SendClientSelection | null =
    context && checked.success && (checked.data.mode === 'existing' || newConfirmed)
      ? {
          taxId: checked.data.mode === 'existing' ? selected?.client.taxId || '' : '',
          ...(checked.data.mode === 'existing'
            ? { clientId: checked.data.clientId }
            : {
                newClient: {
                  name: checked.data.name.trim(),
                  ...(checked.data.email ? { email: checked.data.email } : {}),
                  ...(checked.data.phone ? { phone: checked.data.phone } : {}),
                },
              }),
          tagIds: checked.data.tagIds,
        }
      : null;
  const serialized = JSON.stringify(selection);
  const client = values.mode === 'existing' ? selected?.client : null;
  const contacts = client?.contacts ?? [];
  const phones = [
    ...new Set(
      [
        ...contacts
          .filter((c) => c.kind === 'WHATSAPP')
          .sort((a, b) => Number(b.primary) - Number(a.primary))
          .map((c) => c.value),
        client?.phone || (values.mode === 'new' && newConfirmed ? values.phone : '') || '',
      ].filter(Boolean),
    ),
  ] as string[];
  const emails = [
    ...new Set(
      [
        ...contacts
          .filter((c) => c.kind === 'EMAIL')
          .sort((a, b) => Number(b.primary) - Number(a.primary))
          .map((c) => c.value),
        client?.email || (values.mode === 'new' && newConfirmed ? values.email : '') || '',
      ].filter(Boolean),
    ),
  ] as string[];
  const serializedContacts = JSON.stringify({ phones, emails });
  useEffect(() => {
    if (context && onContactsChange) onContactsChange(JSON.parse(serializedContacts));
  }, [serializedContacts, onContactsChange, context]);

  useEffect(() => {
    onChange(JSON.parse(serialized) as SendClientSelection | null);
  }, [serialized, onChange]);
  useEffect(() => {
    let active = true;
    setFailed(false);
    setContext(null);
    Promise.all([getQuoteClientAssociation(quoteId, branchId), getClientDirectory(branchId)])
      .then(([association, clients]) => {
        if (!active) return;
        const candidates = [
          ...(association.currentClient ? [association.currentClient] : []),
          ...association.suggestions,
          ...clients,
        ];
        const unique = [...new Map(candidates.map((item) => [item.client.id, item])).values()];
        const taxId = association.currentClient?.client.taxId || '';
        const initial = association.currentClient;
        setDirectory(unique);
        setContext(association);
        form.reset({
          mode: 'existing',
          clientId: initial?.client.id || '',
          taxId,
          name: association.contactHints.name || '',
          email: association.contactHints.email || '',
          phone: association.contactHints.phone || '',
          tagIds: initial?.tags.map((tag) => tag.id) || [],
        });
      })
      .catch(() => {
        if (active) {
          setFailed(true);
          toast.error(t('clientLoadFailed'));
        }
      });
    return () => {
      active = false;
    };
  }, [quoteId, branchId, form, loadAttempt, t]);

  function selectClient(id: string) {
    const match = directory.find((item) => item.client.id === id);
    form.setValue('clientId', id, { shouldValidate: true });
    form.setValue('tagIds', match?.tags.map((tag) => tag.id) || []);
    if (match?.client.taxId) form.setValue('taxId', match.client.taxId, { shouldValidate: true });
  }

  if (failed)
    return (
      <Button variant="outline" size="sm" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
        {t('clientRetry')}
      </Button>
    );
  if (!context)
    return (
      <div
        role="status"
        className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted"
      >
        <Spinner size="sm" />
        {t('clientLoading')}
      </div>
    );
  return (
    <Form {...form}>
      <fieldset
        disabled={disabled}
        className="flex min-w-0 flex-col pb-4 gap-y-4 border-b border-border"
      >
        <legend className="mb-3 text-paragraph-sm-medium">{t('clientTitle')}</legend>
        {fixedClient ? (
          <p className="text-paragraph-sm-medium text-foreground">
            {(context.currentClient || selected)?.client.name ||
              (context.currentClient || selected)?.client.email ||
              (context.currentClient || selected)?.client.phone}
          </p>
        ) : (
          <div className="flex items-center justify-between gap-2">
            {values.mode === 'new' && newConfirmed ? (
              <p className="text-paragraph-sm-medium">{values.name}</p>
            ) : (
              <span />
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                form.setValue('mode', 'new');
                setNewOpen(true);
              }}
              disabled={disabled}
            >
              {t('clientNew')}
            </Button>
          </div>
        )}
        {values.mode === 'existing' && !fixedClient ? (
          <FormField
            control={form.control}
            name="clientId"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t('clientTitle')}</FormLabel>
                <FormControl>
                  <Combobox
                    value={field.value}
                    onValueChange={selectClient}
                    placeholder={t('clientChoose')}
                    disabled={disabled || fixedClient}
                    options={directory.map((item) => ({
                      value: item.client.id,
                      label:
                        item.client.name ||
                        item.client.email ||
                        item.client.phone ||
                        t('clientTitle'),
                    }))}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ) : null}
        <Dialog
          open={newOpen}
          onOpenChange={(next) => {
            setNewOpen(next);
            if (!next && !newConfirmed) form.setValue('mode', 'existing', { shouldValidate: true });
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('clientNew')}</DialogTitle>
            </DialogHeader>
            <div className="flex flex-col gap-4">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('clientName')}</FormLabel>
                    <FormControl>
                      <Input {...field} maxLength={255} disabled={disabled} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('clientEmail')}</FormLabel>
                    <FormControl>
                      <Input {...field} type="email" maxLength={255} disabled={disabled} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="phone"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('clientPhone')}</FormLabel>
                    <FormControl>
                      <PhoneInput
                        {...field}
                        disabled={disabled}
                        countryLabel={phoneCopy('country')}
                        countrySearchPlaceholder={phoneCopy('countrySearch')}
                        countryEmptyLabel={phoneCopy('countryEmpty')}
                        landlineNotice={{
                          message: phoneCopy('landline'),
                          action: (mobile) => phoneCopy('useMobile', { number: mobile }),
                        }}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setNewOpen(false);
                  if (!newConfirmed) form.setValue('mode', 'existing', { shouldValidate: true });
                }}
              >
                {t('cancel')}
              </Button>
              <Button
                onClick={async () => {
                  if (await form.trigger(['name', 'phone', 'email'])) {
                    setNewConfirmed(true);
                    setNewOpen(false);
                  }
                }}
              >
                {t('clientUse')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        {!hideTags && context.availableTags.length > 0 ? (
          <div className="flex flex-col gap-y-2">
            <Label>{t('clientTags')}</Label>
            <div className="flex flex-wrap gap-3">
              {context.availableTags.map((tag) => (
                <Label key={tag.id} className="flex items-center gap-x-2">
                  <Checkbox
                    checked={values.tagIds?.includes(tag.id)}
                    disabled={disabled}
                    onCheckedChange={(checked) =>
                      form.setValue(
                        'tagIds',
                        checked
                          ? [...(values.tagIds || []), tag.id]
                          : (values.tagIds || []).filter((id) => id !== tag.id),
                      )
                    }
                  />
                  {tag.name}
                </Label>
              ))}
            </div>
          </div>
        ) : null}
      </fieldset>
    </Form>
  );
}
