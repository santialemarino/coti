'use client';

import { useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Callout,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { resendVerification } from '@/app/(auth)/verify-email/actions';
import {
  resendVerificationSchema,
  type ResendVerificationValues,
} from '@/app/(auth)/verify-email/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

export function ResendVerificationForm() {
  const t = useTranslations('auth.verifyEmail');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.verifyEmail.resend');
  const schema = useMemo(
    () => resendVerificationSchema({ field: t, shared: tErrors }),
    [t, tErrors],
  );
  const [sent, setSent] = useState(false);
  const form = useForm<ResendVerificationValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { email: '' },
  });

  async function onSubmit(values: ResendVerificationValues) {
    const result = await resendVerification(values.email);
    if (result.sent) {
      setSent(true);
      return;
    }
    form.setError(result.field ?? 'root', { message: message(result.error) });
  }

  if (sent) {
    return <Callout tone="success">{t('resend.sent')}</Callout>;
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-5">
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('email.label')}</FormLabel>
              <FormControl>
                <Input
                  type="email"
                  autoComplete="email"
                  maxLength={TEXT_FIELD_MAX_LENGTH}
                  placeholder={t('email.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormRootMessage />

        <PendingButton
          type="submit"
          variant="outline"
          pending={form.formState.isSubmitting}
          pendingLabel={t('resend.submitting')}
        >
          {t('resend.submit')}
        </PendingButton>
      </form>
    </Form>
  );
}
