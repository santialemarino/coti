'use client';

import { useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
} from '@repo/ui/components';
import { changePassword } from '@/app/(protected)/settings/password/actions';
import {
  changePasswordSchema,
  type ChangePasswordValues,
} from '@/app/(protected)/settings/password/form-schema';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

const EMPTY_VALUES: ChangePasswordValues = {
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
};

export function ChangePasswordForm() {
  const t = useTranslations('auth.changePassword');
  const schema = useMemo(() => changePasswordSchema(t), [t]);
  const [done, setDone] = useState(false);
  const form = useForm<ChangePasswordValues>({
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
          : t('newPassword.tooShort');
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
        <FormField
          control={form.control}
          name="currentPassword"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('currentPassword.label')}</FormLabel>
              <FormControl>
                <Input
                  type="password"
                  autoComplete="current-password"
                  placeholder={t('currentPassword.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

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
                  minLength={PASSWORD_MIN_LENGTH}
                  placeholder={t('newPassword.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormDescription>{t('minLength', { count: PASSWORD_MIN_LENGTH })}</FormDescription>
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
                  minLength={PASSWORD_MIN_LENGTH}
                  placeholder={t('confirmPassword.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormRootMessage />
        {done ? <p className="text-paragraph-sm text-foreground-muted">{t('done')}</p> : null}

        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? t('submitting') : t('submit')}
        </Button>
      </form>
    </Form>
  );
}
