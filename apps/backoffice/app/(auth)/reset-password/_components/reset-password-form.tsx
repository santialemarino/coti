'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { CircleCheckIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Card,
  Form,
  FormRootMessage,
  InlineLink,
  PendingButton,
  StatusScreen,
} from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { AuthStage } from '@/app/(auth)/_components/auth-stage';
import { resetPassword } from '@/app/(auth)/reset-password/actions';
import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { PasswordField } from '@/components/password-field';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ResetPasswordFormProps {
  token: string;
}

export function ResetPasswordForm({ token }: ResetPasswordFormProps) {
  const t = useTranslations('auth.resetPassword');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.resetPassword');
  const schema = useMemo(() => resetPasswordSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [done, setDone] = useState(false);
  const form = useForm<ResetPasswordValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { token, newPassword: '', confirmPassword: '' },
  });

  async function onSubmit(values: ResetPasswordValues) {
    const result = await resetPassword(values);
    if (result.done) {
      setDone(true);
      return;
    }
    form.setError(result.field ?? 'root', { message: message(result.error) });
  }

  return (
    <AuthStage stageKey={done ? 'done' : 'form'}>
      {done ? (
        <Card>
          <StatusScreen
            icon={CircleCheckIcon}
            tone="success"
            title={t('title')}
            description={t('done')}
          >
            <InlineLink asChild>
              <Link href={ROUTES.login}>{t('goToLogin')}</Link>
            </InlineLink>
          </StatusScreen>
        </Card>
      ) : (
        <AuthCard title={t('title')}>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              noValidate
              className="flex flex-col gap-y-5"
            >
              <PasswordField
                control={form.control}
                name="newPassword"
                label={t('newPassword.label')}
                placeholder={t('newPassword.placeholder')}
                meter
              />

              <PasswordField
                control={form.control}
                name="confirmPassword"
                label={t('confirmPassword.label')}
                placeholder={t('confirmPassword.placeholder')}
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
