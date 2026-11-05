'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { resetWithRecoveryCode, type RecoveryContact } from '@/app/forgot-password/actions';
import { recoveryCodeSchema } from '@/app/forgot-password/form-schema';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface RecoveryCodeStepProps {
  challengeId: string;
  contact: RecoveryContact;
  minimum: number;
  next: string;
  onResend: () => void;
}

export function RecoveryCodeStep({
  challengeId,
  contact,
  minimum,
  next,
  onResend,
}: RecoveryCodeStepProps) {
  const t = useTranslations('customerRecovery');
  const auth = useTranslations('customerAuth');
  const common = useTranslations('common');
  const form = useForm({
    ...FORM_VALIDATION,
    resolver: zodResolver(
      recoveryCodeSchema(
        (key, values) => (key === 'codeInvalid' ? t(key) : auth(`validation.${key}`, values)),
        minimum,
      ),
    ),
    defaultValues: { code: '', password: '', confirmation: '' },
  });

  async function submit(values: { code: string; password: string }) {
    const result = await resetWithRecoveryCode({
      id: challengeId,
      code: values.code.trim(),
      password: values.password,
      next,
    });
    if (result.error)
      form.setError(result.error === 'code' ? 'code' : 'root', {
        message: t(`errors.${result.error}`),
      });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} noValidate className="flex flex-col gap-y-5">
        <p className="text-paragraph-sm text-foreground-muted">
          {t('codeSent', { contact: contact.masked })}
        </p>
        <FormField
          control={form.control}
          name="code"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('code')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  maxLength={6}
                  autoFocus
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {(['password', 'confirmation'] as const).map((name) => (
          <FormField
            key={name}
            control={form.control}
            name={name}
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{t(name)}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    type="password"
                    maxLength={72}
                    autoComplete="new-password"
                    passwordToggleLabel={common('form.togglePassword')}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ))}
        <FormRootMessage />
        <PendingButton
          type="submit"
          pending={form.formState.isSubmitting}
          pendingLabel={t('saving')}
        >
          {t('save')}
        </PendingButton>
        <InlineLink asChild tone="muted" className="self-center">
          <button type="button" onClick={onResend}>
            {t('resend')}
          </button>
        </InlineLink>
      </form>
    </Form>
  );
}
