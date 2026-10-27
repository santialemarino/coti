'use client';

import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { PlusIcon, Trash2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useFieldArray, useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  Card,
  Combobox,
  ConfirmDialog,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  Label,
  PendingButton,
  PhoneInput,
  RowActionButton,
} from '@repo/ui/components';
import { ContactVerification } from '@/app/profile/_components/contact-verification';
import { saveCustomerProfile } from '@/app/profile/actions';
import { customerProfileSchema, type CustomerProfileValues } from '@/app/profile/form-schema';
import type { ContactKind, CustomerProfile } from '@/lib/api/customer-profile-model';
import { useFormatters } from '@/lib/i18n/formatters';

interface CustomerProfileFormProps {
  profile: CustomerProfile;
}

export function CustomerProfileForm({ profile }: CustomerProfileFormProps) {
  const t = useTranslations('customerProfile');
  const errors = useTranslations('customerProfile.errors');
  const phone = useTranslations('rfq.fields.phone');
  const fmt = useFormatters();
  const saving = useRef(false);
  const router = useRouter();
  const [removing, setRemoving] = useState<{ index: number; value: string } | null>(null);
  const initial = profile.contacts.length ? profile.contacts : [];
  const form = useForm<CustomerProfileValues>({
    resolver: zodResolver(customerProfileSchema(errors)),
    defaultValues: {
      name: profile.name,
      legalName: profile.legalName || '',
      address: profile.address || '',
      ivaCondition: (profile.ivaCondition || '') as
        | ''
        | 'REGISTERED'
        | 'MONOTRIBUTO'
        | 'EXEMPT'
        | 'FINAL_CONSUMER',
      contacts: (['WHATSAPP', 'EMAIL'] as const).flatMap((kind) => {
        const rows = initial.filter((c) => c.kind === kind);
        return rows.length ? rows : [{ kind, value: '', primary: true }];
      }),
    },
  });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'contacts' });
  const values = form.watch('contacts');
  const pending = form.formState.isSubmitting;

  const choosePrimary = (kind: ContactKind, index: number) => {
    values.forEach((contact, i) => {
      if (contact.kind === kind)
        form.setValue(`contacts.${i}.primary`, i === index, { shouldDirty: true });
    });
  };
  const submit = async (data: CustomerProfileValues) => {
    if (saving.current) return;
    saving.current = true;
    let result;
    try {
      result = await saveCustomerProfile(data);
    } catch {
      result = { error: errors('unavailable') };
    } finally {
      saving.current = false;
    }
    if (result.error) {
      form.setError('root', { message: result.error });
      return;
    }
    toast.success(t('saved'));
    form.reset(data);
    router.refresh();
  };

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} noValidate className="flex flex-col gap-y-6">
        {profile.reviewPending ? <Callout>{t('reviewPending')}</Callout> : null}
        <Card className="px-6 gap-y-5">
          <div className="flex flex-col gap-y-2">
            <Label htmlFor="profile-tax-id">{t('taxId')}</Label>
            <Input id="profile-tax-id" value={profile.taxId} readOnly />
          </div>
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t('name')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={t('namePlaceholder')} disabled={pending} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </Card>
        {(['WHATSAPP', 'EMAIL'] as const).map((kind) => (
          <Card key={kind} className="px-6 gap-y-5">
            <h2 className="text-heading-5">{t(kind === 'WHATSAPP' ? 'whatsapp' : 'email')}</h2>
            {fields.map((row, index) =>
              row.kind !== kind ? null : (
                <div key={row.id} className="flex flex-col gap-y-2">
                  <FormField
                    control={form.control}
                    name={`contacts.${index}.value`}
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t('contactNumber', {
                            kind: t(kind === 'WHATSAPP' ? 'whatsapp' : 'email'),
                            number: fields.slice(0, index + 1).filter((c) => c.kind === kind)
                              .length,
                          })}
                        </FormLabel>
                        <div className="flex items-start gap-x-2">
                          <FormControl>
                            {kind === 'WHATSAPP' ? (
                              <PhoneInput
                                {...field}
                                formatters={fmt}
                                disabled={pending}
                                placeholder={t('phonePlaceholder')}
                                countryLabel={phone('country')}
                                countrySearchPlaceholder={phone('search')}
                                countryEmptyLabel={phone('empty')}
                                landlineNotice={{
                                  message: phone('landline'),
                                  action: (mobile) => phone('mobile', { number: mobile }),
                                }}
                              />
                            ) : (
                              <Input
                                {...field}
                                type="email"
                                autoComplete="email"
                                disabled={pending}
                                placeholder={t('emailPlaceholder')}
                              />
                            )}
                          </FormControl>
                          <RowActionButton
                            icon={Trash2Icon}
                            tone="danger"
                            label={t('remove')}
                            disabled={pending}
                            onClick={() => {
                              if (field.value) setRemoving({ index, value: field.value });
                              else remove(index);
                            }}
                          />
                        </div>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  {values[index]?.value ? (
                    <Button
                      variant={values[index]?.primary ? 'secondary' : 'ghost'}
                      size="sm"
                      className="self-start"
                      aria-pressed={values[index]?.primary}
                      disabled={pending}
                      onClick={() => choosePrimary(kind, index)}
                    >
                      {t(values[index]?.primary ? 'primary' : 'makePrimary')}
                    </Button>
                  ) : null}
                  {profile.contacts.some(
                    (c) => c.kind === kind && c.value === values[index]?.value,
                  ) ? (
                    <ContactVerification
                      kind={kind}
                      value={values[index]!.value}
                      verified={profile.contacts.some(
                        (c) => c.kind === kind && c.value === values[index]?.value && c.verified,
                      )}
                    />
                  ) : null}
                </div>
              ),
            )}
            <Button
              variant="outline"
              className="self-start"
              disabled={pending}
              onClick={() =>
                append(
                  {
                    kind,
                    value: '',
                    primary: !values.some((c) => c.kind === kind && c.value !== ''),
                  },
                  { focusName: `contacts.${fields.length}.value` },
                )
              }
            >
              <PlusIcon />
              {t(kind === 'WHATSAPP' ? 'addPhone' : 'addEmail')}
            </Button>
          </Card>
        ))}
        <Card className="px-6 gap-y-5">
          <h2 className="text-heading-5">{t('fiscalTitle')}</h2>
          {(['legalName', 'address'] as const).map((name) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t(name)}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value || ''}
                      maxLength={255}
                      disabled={pending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
          <FormField
            control={form.control}
            name="ivaCondition"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('ivaCondition')}</FormLabel>
                <FormControl>
                  <Combobox
                    value={field.value || ''}
                    onValueChange={field.onChange}
                    resetValue=""
                    placeholder={t('ivaPlaceholder')}
                    options={['', 'REGISTERED', 'MONOTRIBUTO', 'EXEMPT', 'FINAL_CONSUMER'].map(
                      (value) => ({ value, label: t(value ? `iva.${value}` : 'ivaEmpty') }),
                    )}
                    disabled={pending}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </Card>
        <FormRootMessage />
        <PendingButton
          type="submit"
          pending={pending}
          pendingLabel={t('saving')}
          className="self-end"
        >
          {t('save')}
        </PendingButton>
        <ConfirmDialog
          open={removing !== null}
          onOpenChange={(open) => {
            if (!open) setRemoving(null);
          }}
          entity={removing}
          title={t('removeTitle')}
          description={(row) => t('removeDescription', { value: row.value })}
          onConfirm={() => {
            if (removing) remove(removing.index);
            setRemoving(null);
          }}
          labels={{ confirm: t('remove'), pending: t('removing'), cancel: t('cancel') }}
        />
      </form>
    </Form>
  );
}
