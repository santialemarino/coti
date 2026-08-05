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
  FormRootMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { login } from '@/app/(auth)/login/actions';
import { loginSchema, type LoginValues } from '@/app/(auth)/login/form-schema';
import { PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';

interface LoginFormProps {
  next?: string;
}

export function LoginForm({ next }: LoginFormProps) {
  const router = useRouter();
  const t = useTranslations('auth.login');
  const tCommon = useTranslations('common');
  const schema = useMemo(() => loginSchema(t), [t]);
  const form = useForm<LoginValues>({
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
    // Which credential was wrong is deliberately not knowable, so it is a form error.
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
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
                  placeholder={t('email.placeholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name="password"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('password.label')}</FormLabel>
              <FormControl>
                <Input
                  type="password"
                  autoComplete="current-password"
                  minLength={PASSWORD_MIN_LENGTH}
                  placeholder={t('password.placeholder')}
                  passwordToggleLabel={tCommon('form.togglePassword')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
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

        <FormRootMessage />

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
