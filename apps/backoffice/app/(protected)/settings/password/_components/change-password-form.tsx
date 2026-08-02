'use client';

import { useActionState } from 'react';
import { useTranslations } from 'next-intl';

import { Button, FieldError, Input, Label } from '@repo/ui/components';
import {
  changePassword,
  type ChangePasswordState,
} from '@/app/(protected)/settings/password/actions';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

const INITIAL_STATE: ChangePasswordState = {};

export function ChangePasswordForm() {
  const t = useTranslations('auth.changePassword');
  const [state, formAction, pending] = useActionState(changePassword, INITIAL_STATE);

  return (
    <form action={formAction} noValidate className="flex flex-col max-w-sm gap-y-4">
      <div className="flex flex-col gap-y-2">
        <Label htmlFor="currentPassword" required>
          {t('currentPassword.label')}
        </Label>
        <Input
          id="currentPassword"
          name="currentPassword"
          type="password"
          autoComplete="current-password"
          placeholder={t('currentPassword.placeholder')}
          required
        />
      </div>

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
      {state.done ? <p className="text-sm text-muted-foreground">{t('done')}</p> : null}

      <Button type="submit" disabled={pending}>
        {pending ? t('submitting') : t('submit')}
      </Button>
    </form>
  );
}
