'use client';

import { useActionState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button, FieldError, Input, Label } from '@repo/ui/components';
import { login, type LoginState } from '@/app/(auth)/login/actions';
import { ROUTES } from '@/config/routes';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

const INITIAL_STATE: LoginState = {};

interface LoginFormProps {
  next?: string;
}

export function LoginForm({ next }: LoginFormProps) {
  const t = useTranslations('auth.login');
  const [state, formAction, pending] = useActionState(login, INITIAL_STATE);

  return (
    <form action={formAction} noValidate className="flex flex-col gap-y-4">
      <input type="hidden" name="next" value={next ?? ''} />

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

      <div className="flex flex-col gap-y-2">
        <Label htmlFor="password" required>
          {t('password.label')}
        </Label>
        <Input
          id="password"
          name="password"
          type="password"
          autoComplete="current-password"
          minLength={PASSWORD_MIN_LENGTH}
          placeholder={t('password.placeholder')}
          required
        />
      </div>

      <FieldError>{state.error ? t(`errors.${state.error}`) : null}</FieldError>

      <Button type="submit" disabled={pending}>
        {pending ? t('submitting') : t('submit')}
      </Button>

      <Link href={ROUTES.forgotPassword} className="text-sm text-muted-foreground underline">
        {t('forgotPassword')}
      </Link>
    </form>
  );
}
