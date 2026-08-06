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
import { resetPassword } from '@/app/(auth)/reset-password/actions';
import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { ROUTES } from '@/config/routes';
import { PASSWORD_MAX_LENGTH, PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ResetPasswordFormProps {
  token: string;
}

export function ResetPasswordForm({ token }: ResetPasswordFormProps) {
  const t = useTranslations('auth.resetPassword');
  const tCommon = useTranslations('common');
  const tErrors = useTranslations('common.form.errors');
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
    if (result.fieldError) {
      form.setError(result.fieldError.field, {
        message: tErrors('passwordTooShort', { min: PASSWORD_MIN_LENGTH }),
      });
      return;
    }
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
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
              <FormField
                control={form.control}
                name="newPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('newPassword.label')}</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        maxLength={PASSWORD_MAX_LENGTH}
                        placeholder={t('newPassword.placeholder')}
                        passwordToggleLabel={tCommon('form.togglePassword')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="confirmPassword"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('confirmPassword.label')}</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        maxLength={PASSWORD_MAX_LENGTH}
                        placeholder={t('confirmPassword.placeholder')}
                        passwordToggleLabel={tCommon('form.togglePassword')}
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
