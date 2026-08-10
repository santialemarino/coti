'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
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
import { resendVerification } from '@/app/(auth)/verify-email/actions';
import {
  resendVerificationSchema,
  type ResendVerificationValues,
} from '@/app/(auth)/verify-email/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { RESEND_COOLDOWN_SECONDS } from '@/lib/config';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

const A_SECOND = 1000;

/*
 * The form outlives its own success: a mail that never arrives looks exactly like one that was
 * never sent, so the way to ask again has to still be here afterwards. What changes is the button,
 * which shuts for a cooldown — the API's per-address cap answers 202 whether or not it sent
 * anything, so an impatient caller would otherwise spend the allowance on clicks that look like
 * they worked. The cooldown is a courtesy, not the defence: it lives in component state and a
 * reload clears it, while the API's counters are what actually bound the mailbox.
 */
export function ResendVerificationForm() {
  const t = useTranslations('auth.verifyEmail');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.verifyEmail.resend');
  const schema = useMemo(
    () => resendVerificationSchema({ field: t, shared: tErrors }),
    [t, tErrors],
  );
  const opensAt = useRef(0);
  const sending = useRef(false);
  const [remaining, setRemaining] = useState(0);
  const cooling = remaining > 0;
  const form = useForm<ResendVerificationValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { email: '' },
  });

  /*
   * Counted down off the deadline rather than by subtracting a second per tick: a browser throttles
   * timers in a background tab, so a chain of them drifts and the button would stay shut past its
   * time. Keyed on whether it is running at all, not on the number, or the interval would be torn
   * down and rebuilt every second — which is the same drift by another route.
   */
  useEffect(() => {
    if (!cooling) return;
    const ticker = setInterval(
      () => setRemaining(Math.max(0, Math.ceil((opensAt.current - Date.now()) / A_SECOND))),
      A_SECOND,
    );
    return () => clearInterval(ticker);
  }, [cooling]);

  /*
   * Two guards, because they cover different windows. A shut button stops a click but not the Enter
   * key that reaches the form behind it, and `cooling` is a render's value — two submits fired
   * before the first resolves both read it false, which is the one case the cooldown exists to stop.
   * The ref closes that window because it is set in the same tick it is read.
   */
  async function onSubmit(values: ResendVerificationValues) {
    if (cooling || sending.current) return;
    sending.current = true;

    try {
      const result = await resendVerification(values.email);
      if (result.sent) {
        toast.success(t('resend.sent'));
        opensAt.current = Date.now() + RESEND_COOLDOWN_SECONDS * A_SECOND;
        setRemaining(RESEND_COOLDOWN_SECONDS);
        return;
      }
      form.setError(result.field ?? 'root', { message: message(result.error) });
    } finally {
      sending.current = false;
    }
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

        <FormRootMessage />

        {/* The countdown is the label, so the button explains its own silence while it is shut. */}
        <PendingButton
          type="submit"
          variant="outline"
          disabled={cooling}
          pending={form.formState.isSubmitting}
          pendingLabel={t('resend.submitting')}
        >
          {cooling ? t('resend.cooldown', { seconds: remaining }) : t('resend.submit')}
        </PendingButton>
      </form>
    </Form>
  );
}
