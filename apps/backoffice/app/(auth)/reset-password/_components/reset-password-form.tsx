'use client';

import { useActionState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button, FieldError, Input, Label } from '@repo/ui/components';
import { resetPassword, type ResetPasswordState } from '@/app/(auth)/reset-password/actions';
import { ROUTES } from '@/config/routes';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

const INITIAL_STATE: ResetPasswordState = {};

interface ResetPasswordFormProps {
  token: string;
}

export function ResetPasswordForm({ token }: ResetPasswordFormProps) {
  const t = useTranslations('auth.resetPassword');
  const [state, formAction, pending] = useActionState(resetPassword, INITIAL_STATE);

  if (state.done) {
    return (
      <div className="flex flex-col gap-y-4">
        <p className="text-sm text-muted-foreground">{t('done')}</p>
        <Button asChild>
          <Link href={ROUTES.login}>{t('goToLogin')}</Link>
        </Button>
      </div>
    );
  }

  return (
    <form action={formAction} noValidate className="flex flex-col gap-y-4">
      <input type="hidden" name="token" value={token} />

      <div className="flex flex-col gap-y-2">
        <Label htmlFor="newPassword" required>
          {t('newPassword.label')}
        </Label>
        <Input
          id="newPassword"
          name="newPassword"
          type="password"
          autoComplete="new-password"
          minLength={PASSWORD_MIN_LENGTH}
          placeholder={t('newPassword.placeholder')}
          required
        />
        <p className="text-sm text-muted-foreground">
          {t('minLength', { count: PASSWORD_MIN_LENGTH })}
        </p>
      </div>

      <div className="flex flex-col gap-y-2">
        <Label htmlFor="confirmPassword" required>
          {t('confirmPassword.label')}
        </Label>
        <Input
          id="confirmPassword"
          name="confirmPassword"
          type="password"
          autoComplete="new-password"
          minLength={PASSWORD_MIN_LENGTH}
          placeholder={t('confirmPassword.placeholder')}
          required
        />
      </div>

      <FieldError>{state.error ? t(`errors.${state.error}`) : null}</FieldError>

      <Button type="submit" disabled={pending}>
        {pending ? t('submitting') : t('submit')}
      </Button>
    </form>
  );
}
