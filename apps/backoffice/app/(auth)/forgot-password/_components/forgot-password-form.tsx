'use client';

import { useActionState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button, FieldError, Input, Label } from '@repo/ui/components';
import {
  requestPasswordRecovery,
  type ForgotPasswordState,
} from '@/app/(auth)/forgot-password/actions';
import { ROUTES } from '@/config/routes';

const INITIAL_STATE: ForgotPasswordState = {};

export function ForgotPasswordForm() {
  const t = useTranslations('auth.forgotPassword');
  const [state, formAction, pending] = useActionState(requestPasswordRecovery, INITIAL_STATE);

  if (state.sent) {
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
    <form action={formAction} noValidate className="flex flex-col gap-y-4">
      <div className="flex flex-col gap-y-2">
        <Label htmlFor="email" required>
          {t('email.label')}
        </Label>
        <Input
          id="email"
          name="email"
          type="email"
          autoComplete="email"
          defaultValue={state.email}
          placeholder={t('email.placeholder')}
          required
        />
      </div>

      <FieldError>{state.error ? t(`errors.${state.error}`) : null}</FieldError>

      <Button type="submit" disabled={pending}>
        {pending ? t('submitting') : t('submit')}
      </Button>

      <Link href={ROUTES.login} className="text-sm text-muted-foreground underline">
        {t('backToLogin')}
      </Link>
    </form>
  );
}
