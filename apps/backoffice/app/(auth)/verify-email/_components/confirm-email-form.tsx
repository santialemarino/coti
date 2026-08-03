'use client';

import { useActionState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { confirmEmail, type ConfirmEmailResult } from '@/app/(auth)/verify-email/actions';
import { ROUTES } from '@/config/routes';

const INITIAL_STATE: ConfirmEmailResult = {};

interface ConfirmEmailFormProps {
  token: string;
}

/*
 * Confirming is a button rather than something that happens on load. The link is single use,
 * and a mail client's scanner, a corporate link checker or a router prefetch will all issue a
 * GET — any of which would burn the token before the person reading the mail ever clicked.
 */
export function ConfirmEmailForm({ token }: ConfirmEmailFormProps) {
  const t = useTranslations('auth.verifyEmail');
  const [state, formAction, pending] = useActionState(confirmEmail, INITIAL_STATE);

  if (state.done) {
    return (
      <div className="flex flex-col gap-y-4">
        <p className="text-sm text-muted-foreground">{t('done')}</p>
        <Button asChild>
          <Link href={ROUTES.home}>{t('continue')}</Link>
        </Button>
      </div>
    );
  }

  if (state.error) {
    return (
      <div className="flex flex-col gap-y-4">
        <p className="text-sm text-destructive">{t(`errors.${state.error}`)}</p>
        <p className="text-sm text-muted-foreground">{t('resendHint')}</p>
        <ResendVerificationForm />
      </div>
    );
  }

  return (
    <form action={formAction} className="flex flex-col gap-y-4">
      <input type="hidden" name="token" value={token} />
      <p className="text-sm text-muted-foreground">{t('prompt')}</p>
      <Button type="submit" disabled={pending}>
        {pending ? t('confirming') : t('confirm')}
      </Button>
    </form>
  );
}
