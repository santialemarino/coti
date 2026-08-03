'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
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
import { requestPasswordRecovery } from '@/app/(auth)/forgot-password/actions';
import {
  forgotPasswordSchema,
  type ForgotPasswordValues,
} from '@/app/(auth)/forgot-password/form-schema';
import { ROUTES } from '@/config/routes';

export function ForgotPasswordForm() {
  const t = useTranslations('auth.forgotPassword');
  const schema = useMemo(() => forgotPasswordSchema(t), [t]);
  const [sent, setSent] = useState(false);
  const form = useForm<ForgotPasswordValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: '' },
  });

  async function onSubmit(values: ForgotPasswordValues) {
    const result = await requestPasswordRecovery(values);
    if (result.sent) {
      setSent(true);
      return;
    }
    if (result.fieldError) {
      form.setError(result.fieldError.field, { message: t(`email.${result.fieldError.key}`) });
      return;
    }
    form.setError('root', { message: t('errors.unexpected') });
  }

  if (sent) {
    return (
      <div className="flex flex-col gap-y-4">
        <p className="text-sm text-muted-foreground">{t('sent')}</p>
        <Button asChild variant="outline">
          <Link href={ROUTES.login}>{t('backToLogin')}</Link>
        </Button>
      </div>
    );
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

        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? t('submitting') : t('submit')}
        </Button>

        <Link href={ROUTES.login} className="text-sm text-muted-foreground underline">
          {t('backToLogin')}
        </Link>
      </form>
    </Form>
  );
}
