'use client';

import { useMemo } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import { Form, FormRootMessage, PendingButton } from '@repo/ui/components';
import { changePassword } from '@/app/(protected)/settings/password/actions';
import {
  changePasswordSchema,
  type ChangePasswordValues,
} from '@/app/(protected)/settings/password/form-schema';
import { PasswordField } from '@/components/password-field';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { FORM_VALIDATION } from '@/lib/forms/options';

const EMPTY_VALUES: ChangePasswordValues = {
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
};

export function ChangePasswordForm() {
  const t = useTranslations('auth.changePassword');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.changePassword');
  const schema = useMemo(() => changePasswordSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<ChangePasswordValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: EMPTY_VALUES,
  });

  async function onSubmit(values: ChangePasswordValues) {
    const result = await changePassword(values);
    if (result.done) {
      toast.success(t('done'));
      // Nothing is left to resubmit, and a password should not sit in the DOM.
      form.reset(EMPTY_VALUES);
      return;
    }
    form.setError(result.field ?? 'root', { message: message(result.error) });
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
