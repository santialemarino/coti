'use client';

import { useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
  Separator,
} from '@repo/ui/components';
import { updateAccount } from '@/app/(protected)/settings/account/actions';
import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import { BrandColorField } from '@/components/brand-color-field';
import { LogoDropzone } from '@/components/logo-dropzone';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Account } from '@/lib/api/account';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface AccountFormProps {
  account: Account;
}

export function AccountForm({ account }: AccountFormProps) {
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

  async function onSubmit(values: AccountValues) {
    const result = await updateAccount(values, logo);
    if (result.ok) {
      toast.success(t('saved'));
      return;
    }
    // The rejection belongs to the form, not to a field: the two the API answers with are both
    // values this schema already refuses.
    form.setError('root', { message: message(result.error) });
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        noValidate
        className="flex flex-col max-w-md gap-y-5"
      >
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
              <FormMessage />
            </FormItem>
          )}
        />

        <Separator />

        <h2 className="text-heading-6">{t('brand.title')}</h2>

        <LogoDropzone initialUrl={account.brandLogoUrl} onFileChange={setLogo} />

        <BrandColorField
          control={form.control}
          name="brandColor"
          label={t('brandColor.label')}
          placeholder={t('brandColor.placeholder')}
          pickerLabel={t('brandColor.pickerLabel')}
        />

        <FormRootMessage />

        <PendingButton
          type="submit"
          className="self-start"
          pending={form.formState.isSubmitting}
          pendingLabel={t('submitting')}
        >
          {t('submit')}
        </PendingButton>
      </form>
    </Form>
  );
}
