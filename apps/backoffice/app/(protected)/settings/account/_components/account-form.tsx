'use client';

import { useMemo } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
  Separator,
} from '@repo/ui/components';
import { updateAccount } from '@/app/(protected)/settings/account/actions';
import { accountSchema, type AccountValues } from '@/app/(protected)/settings/account/form-schema';
import type { Account } from '@/lib/api/account';
import { HEX_COLOR } from '@/lib/constants/brand';
import { TEXT_FIELD_MAX_LENGTH, URL_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface AccountFormProps {
  account: Account;
}

export function AccountForm({ account }: AccountFormProps) {
  const t = useTranslations('account');
  const tErrors = useTranslations('common.form.errors');
  const schema = useMemo(() => accountSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<AccountValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: {
      name: account.name,
      legalName: account.legalName ?? '',
      taxId: account.taxId ?? '',
      brandLogoUrl: account.brandLogoUrl ?? '',
      brandColor: account.brandColor ?? '',
    },
  });
  const brandColor = useWatch({ control: form.control, name: 'brandColor' });
  const brandLogoUrl = useWatch({ control: form.control, name: 'brandLogoUrl' });
  // Previewed only once the value is one the API would store, so the swatch going blank is the
  // first thing that says a colour is malformed.
  const swatch = HEX_COLOR.test(brandColor) ? brandColor : null;
  const logo = URL.canParse(brandLogoUrl) ? brandLogoUrl : null;

  async function onSubmit(values: AccountValues) {
    const result = await updateAccount(values);
    if (result.ok) {
      toast.success(t('saved'));
      return;
    }
    // The rejection belongs to the form, not to a field: the two the API answers with are both
    // values this schema already refuses.
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
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
              <FormDescription>{t('name.hint')}</FormDescription>
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

        <div className="flex flex-col gap-y-1">
          <h2 className="text-heading-6">{t('brand.title')}</h2>
          <p className="text-paragraph-sm text-foreground-muted">{t('brand.description')}</p>
        </div>

        <FormField
          control={form.control}
          name="brandLogoUrl"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('brandLogoUrl.label')}</FormLabel>
              <FormControl>
                <Input
                  type="url"
                  inputMode="url"
                  maxLength={URL_FIELD_MAX_LENGTH}
                  placeholder={t('brandLogoUrl.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormDescription>
                {t('brandLogoUrl.hint')}{' '}
                {/* Opened rather than rendered: the backoffice does not load an address someone
                    pasted, and one click confirms it is the right image. */}
                {logo ? (
                  <InlineLink asChild>
                    <a href={logo} target="_blank" rel="noreferrer noopener">
                      {t('brandLogoUrl.open')}
                    </a>
                  </InlineLink>
                ) : null}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="brandColor"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('brandColor.label')}</FormLabel>
              <div className="flex items-center gap-x-2.5">
                <FormControl>
                  <Input placeholder={t('brandColor.placeholder')} {...field} />
                </FormControl>
                {/* The account's own colour is data, not styling, so no token can express it. The
                    class keeps the box occupying its space while there is nothing to show. */}
                <span
                  data-slot="brand-swatch"
                  aria-hidden="true"
                  className="shrink-0 size-9 bg-input-readonly border border-border rounded-lg"
                  style={swatch ? { backgroundColor: swatch } : undefined}
                />
              </div>
              <FormDescription>{t('brandColor.hint')}</FormDescription>
              <FormMessage />
            </FormItem>
          )}
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
