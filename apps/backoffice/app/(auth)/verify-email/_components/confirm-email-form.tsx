'use client';

import { useActionState } from 'react';
import Link from 'next/link';
import { CircleCheckIcon, CircleXIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Card, Hint, InlineLink, PendingButton, StatusScreen } from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { AuthStage } from '@/app/(auth)/_components/auth-stage';
import { ResendVerificationForm } from '@/app/(auth)/verify-email/_components/resend-verification-form';
import { confirmEmail, type ConfirmEmailResult } from '@/app/(auth)/verify-email/actions';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';

const INITIAL_STATE: ConfirmEmailResult = {};

interface ConfirmEmailFormProps {
  token: string;
  /*
   * The caller's own address, when a session names one — which also answers whether there is
   * anywhere to go back to. Confirming happens in whichever browser the mail client opened, and
   * that one usually holds no session, so home would only bounce off the gate.
   */
  address?: string;
}

/*
 * Confirming is a button rather than something that happens on load. The link is single use,
 * and a mail client's scanner, a corporate link checker or a router prefetch will all issue a
 * GET — any of which would burn the token before the person reading the mail ever clicked.
 */
export function ConfirmEmailForm({ token, address }: ConfirmEmailFormProps) {
  const t = useTranslations('auth.verifyEmail');
  const message = useApiErrorMessage('auth.verifyEmail');
  const [state, formAction, pending] = useActionState(confirmEmail, INITIAL_STATE);

  const stage = state.done ? 'done' : state.error ? 'error' : 'prompt';

  return (
    <AuthStage stageKey={stage}>
      {state.done ? (
        <Card>
          <StatusScreen
            icon={CircleCheckIcon}
            tone="success"
            title={t('title')}
            description={t('done')}
          >
            <InlineLink asChild>
              <Link href={address ? ROUTES.home : ROUTES.login}>
                {address ? t('continue') : t('goToLogin')}
              </Link>
            </InlineLink>
          </StatusScreen>
        </Card>
      ) : state.error ? (
        <Card className="gap-y-6">
          <StatusScreen
            icon={CircleXIcon}
            tone="danger"
            title={t('title')}
            description={message(state.error)}
          />
          <div className="flex flex-col px-6 gap-y-4">
            <Hint>{address ? t('resend.hintKnown') : t('resend.hint')}</Hint>
            <ResendVerificationForm address={address} />
          </div>
        </Card>
      ) : (
        <AuthCard
          title={t('title')}
          description={t('prompt')}
          footer={
            <InlineLink asChild tone="muted">
              <Link href={address ? ROUTES.home : ROUTES.login}>
                {address ? t('continue') : t('backToLogin')}
              </Link>
            </InlineLink>
          }
        >
          <form action={formAction} className="flex flex-col">
            <input type="hidden" name="token" value={token} />
            <PendingButton type="submit" size="lg" pending={pending} pendingLabel={t('confirming')}>
              {t('confirm')}
            </PendingButton>
          </form>
        </AuthCard>
      )}
    </AuthStage>
  );
}
