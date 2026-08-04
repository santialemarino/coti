'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { MailCheckIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Card,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  StatusScreen,
} from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { AuthStage } from '@/app/(auth)/_components/auth-stage';
import { requestPasswordRecovery } from '@/app/(auth)/forgot-password/actions';
import {
  forgotPasswordSchema,
  type ForgotPasswordValues,
} from '@/app/(auth)/forgot-password/form-schema';
import { ROUTES } from '@/config/routes';

/*
 * Owns the whole screen rather than just the form, because the sent state replaces the card entirely
 * — title included — and the crossfade has to wrap both stages to animate between them.
 */
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

  return (
    <AuthStage stageKey={sent ? 'sent' : 'form'}>
      {sent ? (
        <Card>
          <StatusScreen icon={MailCheckIcon} tone="info" title={t('title')} description={t('sent')}>
            <InlineLink asChild>
              <Link href={ROUTES.login}>{t('backToLogin')}</Link>
            </InlineLink>
          </StatusScreen>
        </Card>
      ) : (
        <AuthCard
          title={t('title')}
          description={t('description')}
          footer={
            <InlineLink asChild tone="muted">
              <Link href={ROUTES.login}>{t('backToLogin')}</Link>
            </InlineLink>
          }
        >
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              noValidate
              className="flex flex-col gap-y-5"
            >
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

              <Button type="submit" size="lg" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? t('submitting') : t('submit')}
              </Button>
            </form>
          </Form>
        </AuthCard>
      )}
    </AuthStage>
  );
}
