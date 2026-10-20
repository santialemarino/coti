'use client';

import { useEffect, useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { isValidPhoneNumber } from 'libphonenumber-js/min';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';

import {
  Callout,
  Checkbox,
  Combobox,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  Label,
  Spinner,
  TaxIdInput,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { isValidCuit } from '@repo/ui/lib';
import { PhoneInput } from '@/components/phone-input';
import type { ClientMatch, QuoteClientAssociation } from '@/lib/api/client-profiles';
import { getClientDirectory, getQuoteClientAssociation } from '@/lib/api/clients-client';
import type { SendClientSelection } from '@/lib/api/rfqs';

interface SendClientFormProps {
  quoteId: string;
  branchId: string;
  disabled: boolean;
  locked?: boolean;
  onChange: (selection: SendClientSelection | null) => void;
}

export function SendClientForm({
  quoteId,
  branchId,
  disabled,
  locked = false,
  onChange,
}: SendClientFormProps) {
  const t = useTranslations('rfqs.detail.send');
  const phoneCopy = useTranslations('common.phone');
  const [context, setContext] = useState<QuoteClientAssociation | null>(null);
  const [directory, setDirectory] = useState<ClientMatch[]>([]);
  const [failed, setFailed] = useState(false);
  const schema = useMemo(
    () =>
      z
        .object({
          mode: z.enum(['existing', 'new']),
          clientId: z.string(),
          taxId: z
            .string()
            .refine(
              (value) => isValidCuit(value) && value !== '00000000000',
              t('clientTaxInvalid'),
            ),
          name: z.string().max(255),
          email: z.string().trim(),
          phone: z.string(),
          tagIds: z.array(z.string()),
        })
        .superRefine((values, ctx) => {
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
      mode: 'new',
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
  const fixedClient = locked && context?.currentClient != null;
  const mismatch =
    values.mode === 'existing' && selected?.client.taxId && selected.client.taxId !== values.taxId;
  const checked = schema.safeParse(values);
  const selection: SendClientSelection | null =
    context && checked.success && !mismatch
      ? {
          taxId: checked.data.taxId,
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

  useEffect(() => {
    onChange(JSON.parse(serialized) as SendClientSelection | null);
  }, [serialized, onChange]);
  useEffect(() => {
    let active = true;
    Promise.all([getQuoteClientAssociation(quoteId, branchId), getClientDirectory(branchId)])
      .then(([association, clients]) => {
        if (!active) return;
        const candidates = [
          ...(association.currentClient ? [association.currentClient] : []),
          ...association.suggestions,
          ...clients,
        ];
        const unique = [...new Map(candidates.map((item) => [item.client.id, item])).values()];
        const taxId =
          association.currentClient?.client.taxId || association.contactHints.taxId || '';
        const matches = unique.filter((item) => taxId && item.client.taxId === taxId);
        const initial =
          association.currentClient || (matches.length === 1 ? matches[0] : undefined);
        setDirectory(unique);
        setContext(association);
        form.reset({
          mode: initial || matches.length > 0 ? 'existing' : 'new',
          clientId: initial?.client.id || '',
          taxId,
          name: association.contactHints.name || '',
          email: association.contactHints.email || '',
          phone: association.contactHints.phone || '',
          tagIds: initial?.tags.map((tag) => tag.id) || [],
        });
      })
      .catch(() => {
        if (active) setFailed(true);
      });
    return () => {
      active = false;
    };
  }, [quoteId, branchId, form]);

  function selectClient(id: string) {
    const match = directory.find((item) => item.client.id === id);
    form.setValue('clientId', id, { shouldValidate: true });
    form.setValue('tagIds', match?.tags.map((tag) => tag.id) || []);
    if (match?.client.taxId) form.setValue('taxId', match.client.taxId, { shouldValidate: true });
  }

  function enterTaxId(value: string) {
    form.setValue('taxId', value, { shouldValidate: true });
    const matches = directory.filter((item) => item.client.taxId === value);
    if (!context?.currentClient && isValidCuit(value) && matches.length > 0) {
      form.setValue('mode', 'existing');
      form.setValue('clientId', matches.length === 1 ? matches[0]!.client.id : '');
      form.setValue('tagIds', matches.length === 1 ? matches[0]!.tags.map((tag) => tag.id) : []);
    }
  }

  if (failed) return <Callout tone="danger">{t('clientLoadFailed')}</Callout>;
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
        <ToggleGroup
          type="single"
          value={values.mode}
          onValueChange={(mode) => {
            if (mode === 'new' || mode === 'existing')
              form.setValue('mode', mode, { shouldValidate: true });
          }}
          className="w-full"
          disabled={disabled || fixedClient}
        >
          <ToggleGroupItem value="existing" className="flex-1">
            {t('clientExisting')}
          </ToggleGroupItem>
          <ToggleGroupItem value="new" className="flex-1">
            {t('clientNew')}
          </ToggleGroupItem>
        </ToggleGroup>
        {values.mode === 'existing' ? (
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
                      label: [
                        item.client.name || item.client.email || item.client.phone,
                        item.client.taxId,
                      ]
                        .filter(Boolean)
                        .join(' · '),
                    }))}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ) : null}
        <FormField
          control={form.control}
          name="taxId"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('clientTax')}</FormLabel>
              <FormControl>
                <TaxIdInput
                  {...field}
                  onChange={enterTaxId}
                  kind="cuit"
                  disabled={disabled}
                  readOnly={fixedClient && !!selected?.client.taxId}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {mismatch ? <Callout tone="danger">{t('clientTaxMismatch')}</Callout> : null}
        {values.mode === 'new' ? (
          <>
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
          </>
        ) : null}
        {context.availableTags.length > 0 ? (
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
