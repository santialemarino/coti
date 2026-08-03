'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
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
import { resetPassword } from '@/app/(auth)/reset-password/actions';
import {
  resetPasswordSchema,
  type ResetPasswordValues,
} from '@/app/(auth)/reset-password/form-schema';
import { ROUTES } from '@/config/routes';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

interface ResetPasswordFormProps {
  token: string;
}

export function ResetPasswordForm({ token }: ResetPasswordFormProps) {
  const t = useTranslations('auth.resetPassword');
  const schema = useMemo(() => resetPasswordSchema(t), [t]);
  const [done, setDone] = useState(false);
  const form = useForm<ResetPasswordValues>({
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
      form.setError(result.fieldError.field, { message: t('newPassword.tooShort') });
      return;
    }
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
  }

  if (done) {
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
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-4">
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

        <Button type="submit" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? t('submitting') : t('submit')}
        </Button>
      </form>
    </Form>
  );
}
