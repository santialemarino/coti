'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { PasswordField } from '@/components/password-field';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { changeEmail } from '@/lib/auth/change-email';
import { changeEmailSchema, type ChangeEmailValues } from '@/lib/auth/change-email-schema';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

const EMPTY_VALUES: ChangeEmailValues = { newEmail: '', currentPassword: '' };

/*
 * The same form on both surfaces it belongs on: the confirmation screen, which is where an
 * unconfirmed caller is sent, and settings, where a confirmed one looks for it. It carries no
 * copy of the current address — the screen around it already names that.
 */
export function ChangeEmailForm() {
  const router = useRouter();
  const t = useTranslations('auth.changeEmail');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.changeEmail');
  const schema = useMemo(() => changeEmailSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<ChangeEmailValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: EMPTY_VALUES,
  });

  async function onSubmit(values: ChangeEmailValues) {
    const result = await changeEmail(values);
    if (result.done) {
      toast.success(t('done'));
      form.reset(EMPTY_VALUES);
      // The address is confirmed nowhere now, so where this caller belongs has changed: only a
      // re-render asks the protected layout again, and it names the new address.
      router.refresh();
      return;
    }
    form.setError(result.field ?? 'root', { message: message(result.error) });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-4">
        <FormField
          control={form.control}
          name="newEmail"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('newEmail.label')}</FormLabel>
              <FormControl>
                <Input
                  type="email"
                  autoComplete="email"
                  maxLength={TEXT_FIELD_MAX_LENGTH}
                  placeholder={t('newEmail.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <PasswordField
          control={form.control}
          name="currentPassword"
          label={t('currentPassword.label')}
          placeholder={t('currentPassword.placeholder')}
          existing
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
