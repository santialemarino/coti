'use client';

import { useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import { Form, FormRootMessage, PendingButton } from '@repo/ui/components';
import { changePassword } from '@/app/(protected)/settings/password/actions';
import {
  changePasswordSchema,
  type ChangePasswordValues,
} from '@/app/(protected)/settings/password/form-schema';
import { PasswordField } from '@/components/password-field';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/password';
import { FORM_VALIDATION } from '@/lib/forms/options';

const EMPTY_VALUES: ChangePasswordValues = {
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
};

export function ChangePasswordForm() {
  const t = useTranslations('auth.changePassword');
  const tErrors = useTranslations('common.form.errors');
  const schema = useMemo(() => changePasswordSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [done, setDone] = useState(false);
  const form = useForm<ChangePasswordValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: EMPTY_VALUES,
  });

  async function onSubmit(values: ChangePasswordValues) {
    setDone(false);
    const result = await changePassword(values);
    if (result.done) {
      setDone(true);
      // Nothing is left to resubmit, and a password should not sit in the DOM.
      form.reset(EMPTY_VALUES);
      return;
    }
    if (result.fieldError) {
      const message =
        result.fieldError.key === 'wrong'
          ? t('errors.wrongCurrentPassword')
          : tErrors('passwordTooShort', { min: PASSWORD_MIN_LENGTH });
      form.setError(result.fieldError.field, { message });
      return;
    }
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        noValidate
        className="flex flex-col max-w-sm gap-y-4"
      >
        <PasswordField
          control={form.control}
          name="currentPassword"
          label={t('currentPassword.label')}
          placeholder={t('currentPassword.placeholder')}
          existing
        />

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
        {done ? <p className="text-paragraph-sm text-foreground-muted">{t('done')}</p> : null}

        <PendingButton
          type="submit"
          pending={form.formState.isSubmitting}
          pendingLabel={t('submitting')}
        >
          {t('submit')}
        </PendingButton>
      </form>
    </Form>
  );
}
