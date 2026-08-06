'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Checkbox,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { login } from '@/app/(auth)/login/actions';
import { loginSchema, type LoginValues } from '@/app/(auth)/login/form-schema';
import { PasswordField } from '@/components/password-field';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface LoginFormProps {
  next?: string;
}

export function LoginForm({ next }: LoginFormProps) {
  const router = useRouter();
  const t = useTranslations('auth.login');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.login');
  const schema = useMemo(() => loginSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<LoginValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '', rememberMe: false },
  });

  async function onSubmit(values: LoginValues) {
    const result = await login(values, next);
    if (result.redirectTo) {
      router.replace(result.redirectTo);
      // The session cookie only exists as of this response, so the cached tree still
      // reflects an anonymous caller.
      router.refresh();
      return;
    }
    /*
     * On the password rather than the form: which credential was wrong stays unknowable, but the
     * message has to clear when the caller edits their attempt, and a root error only clears on the
     * next submit — leaving a stale rejection under a corrected password.
     */
    form.setError('password', { message: message(result.error) });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-5">
        <FormField
          control={form.control}
          name="email"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('email.label')}</FormLabel>
              <FormControl>
                <Input
                  type="email"
                  autoComplete="email"
                  maxLength={TEXT_FIELD_MAX_LENGTH}
                  placeholder={t('email.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <PasswordField
          control={form.control}
          name="password"
          label={t('password.label')}
          placeholder={t('password.placeholder')}
          existing
        />

        <FormField
          control={form.control}
          name="rememberMe"
          render={({ field }) => (
            <FormItem className="flex-row items-center gap-x-2">
              <FormControl>
                <Checkbox checked={field.value} onCheckedChange={field.onChange} />
              </FormControl>
              <FormLabel className="cursor-pointer text-paragraph-sm text-foreground-muted">
                {t('rememberMe')}
              </FormLabel>
            </FormItem>
          )}
        />

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
  );
}
