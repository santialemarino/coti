'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  ConfirmDialog,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { updateAccount } from '@/app/(protected)/settings/account/actions';
import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { BrandColorField } from '@/components/brand-color-field';
import { LogoDropzone } from '@/components/logo-dropzone';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Account } from '@/lib/api/account';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface AccountFormProps {
  account: Account;
  // Where the ARCA connection made with this CUIT stands; null when there is none.
  arcaConnection: 'inProgress' | 'connected' | null;
}

export function AccountForm({ account, arcaConnection }: AccountFormProps) {
  const t = useTranslations('account');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('account');
  const schema = useMemo(() => accountSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<AccountValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: {
      name: account.name,
      legalName: account.legalName ?? '',
      taxId: account.taxId ?? '',
      brandLogoUrl: account.brandLogoUrl ?? '',
      brandColor: account.brandColor?.replace(/^#/, '') ?? '',
    },
  });
  const [logo, setLogo] = useState<File | null>();
  // The values a CUIT change is waiting on while the caller decides about the ARCA connection.
  const [confirming, setConfirming] = useState<AccountValues | null>(null);
  const [disconnecting, setDisconnecting] = useState(false);
  const [invoicePending, setInvoicePending] = useState(false);
  const heldConnection = useHeldWhileClosed(arcaConnection, confirming !== null);

  async function save(values: AccountValues, disconnectARCA: boolean) {
    setInvoicePending(false);
    const result = await updateAccount(values, logo, disconnectARCA);
    if (result.ok) {
      toast.success(t(disconnectARCA ? 'savedDisconnected' : 'saved'));
      return;
    }
    // A CUIT change ARCA is connected with asks first, then goes again with the confirmation.
    if (result.error === 'ARCA_CONNECTED') {
      setConfirming(values);
      return;
    }
    // The refusals a field owns; the rest are values this schema already refuses.
    if (result.error === 'INVOICE_PENDING') {
      setInvoicePending(true);
      form.setError('taxId', { message: message(result.error) }, { shouldFocus: true });
      return;
    }
    form.setError('root', { message: message(result.error) });
  }

  async function confirmDisconnect() {
    if (!confirming) return;
    setDisconnecting(true);
    try {
      await save(confirming, true);
    } finally {
      setDisconnecting(false);
      setConfirming(null);
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((values) => save(values, false))}
        noValidate
        className="flex flex-col w-full gap-y-6 lg:gap-y-8"
      >
        {/* One column as narrow as a field reads well, or two filling the page once it is wide
            enough; the save button spans both because it saves both. */}
        <div className="grid grid-cols-1 gap-x-12 gap-y-8 lg:grid-cols-2">
          <section className="flex flex-col max-w-md gap-y-5 lg:max-w-none">
            <h2 className="text-heading-6">{t('company.title')}</h2>

            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('name.label')}</FormLabel>
                  <FormControl>
                    <Input
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('name.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="legalName"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('legalName.label')}</FormLabel>
                  <FormControl>
                    <Input
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('legalName.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="taxId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('taxId.label')}</FormLabel>
                  <FormControl>
                    <Input
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('taxId.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage>
                    {invoicePending && form.formState.errors.taxId ? (
                      <>
                        {t('invoicePending')}{' '}
                        <InlineLink asChild>
                          <Link href={ROUTES.administration}>{t('invoicePendingLink')}</Link>
                        </InlineLink>
                      </>
                    ) : undefined}
                  </FormMessage>
                </FormItem>
              )}
            />
          </section>

          <section className="flex flex-col max-w-md gap-y-5 lg:max-w-none">
            <h2 className="text-heading-6">{t('brand.title')}</h2>

            <LogoDropzone initialUrl={account.brandLogoUrl} onFileChange={setLogo} />

            <BrandColorField
              control={form.control}
              name="brandColor"
              label={t('brandColor.label')}
              placeholder={t('brandColor.placeholder')}
              pickerLabel={t('brandColor.pickerLabel')}
            />
          </section>
        </div>

        <FormRootMessage />

        <PendingButton
          type="submit"
          className="w-full max-w-md lg:max-w-none"
          pending={form.formState.isSubmitting}
          pendingLabel={t('submitting')}
        >
          {t('submit')}
        </PendingButton>
      </form>
      <ConfirmDialog
        open={confirming !== null}
        onOpenChange={(open) => !open && setConfirming(null)}
        entity={account.taxId}
        title={t('disconnectARCA.title')}
        description={(cuit) =>
          t(
            heldConnection === 'inProgress'
              ? 'disconnectARCA.inProgress'
              : 'disconnectARCA.connected',
            { old: cuit },
          )
        }
        onConfirm={confirmDisconnect}
        pending={disconnecting}
        labels={{
          confirm: t('disconnectARCA.confirm'),
          pending: t('submitting'),
          cancel: t('disconnectARCA.cancel'),
        }}
      />
    </Form>
  );
}
