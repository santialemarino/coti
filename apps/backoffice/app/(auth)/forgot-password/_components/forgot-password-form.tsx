'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { MailCheckIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
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
  PendingButton,
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
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

/*
 * Owns the whole screen rather than just the form, because the sent state replaces the card entirely
 * — title included — and the crossfade has to wrap both stages to animate between them.
 */
export function ForgotPasswordForm() {
  const t = useTranslations('auth.forgotPassword');
  const tErrors = useTranslations('common.form.errors');
  const schema = useMemo(() => forgotPasswordSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [sent, setSent] = useState(false);
  const form = useForm<ForgotPasswordValues>({
    ...FORM_VALIDATION,
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
      form.setError(result.fieldError.field, { message: tErrors('invalidEmail') });
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
                size="lg"
                pending={form.formState.isSubmitting}
                pendingLabel={t('submitting')}
              >
                {t('submit')}
              </PendingButton>
            </form>
          </Form>
        </AuthCard>
      )}
    </AuthStage>
  );
}
