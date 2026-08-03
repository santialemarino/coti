'use client';

import { useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
} from '@repo/ui/components';
import { resendVerification } from '@/app/(auth)/verify-email/actions';
import {
  resendVerificationSchema,
  type ResendVerificationValues,
} from '@/app/(auth)/verify-email/form-schema';

export function ResendVerificationForm() {
  const t = useTranslations('auth.verifyEmail');
  const schema = useMemo(() => resendVerificationSchema(t), [t]);
  const [sent, setSent] = useState(false);
  const form = useForm<ResendVerificationValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: '' },
  });

  async function onSubmit(values: ResendVerificationValues) {
    const result = await resendVerification(values.email);
    if (result.sent) {
      setSent(true);
      return;
    }
    if (result.error === 'invalidEmail') {
      form.setError('email', { message: t('email.invalid') });
      return;
    }
    form.setError('root', { message: t('errors.unexpected') });
  }

  if (sent) {
    return <p className="text-sm text-muted-foreground">{t('resent')}</p>;
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-4">
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
                  placeholder={t('email.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormRootMessage />

        <Button type="submit" variant="outline" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? t('resending') : t('resend')}
        </Button>
      </form>
    </Form>
  );
}
