'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import type { ARCASetup } from '@/lib/api/arca-setup';
import { formatCuit } from '@/lib/api/invoicing';
import { useFormatters } from '@/lib/i18n/formatters';

export type ARCAConnectionState =
  | 'disabled'
  | 'notConnected'
  | 'inProgress'
  | 'connected'
  | 'expired';

const BADGE_TONE = {
  disabled: 'neutral',
  notConnected: 'neutral',
  inProgress: 'warning',
  connected: 'success',
  expired: 'danger',
} as const;

const ACTION = {
  notConnected: 'start',
  inProgress: 'continue',
  connected: 'manage',
  expired: 'manage',
} as const;

// Where the connection stands, from the account's setup and the date the page was rendered on.
export function arcaConnectionState(setup: ARCASetup, now: string): ARCAConnectionState {
  if (!setup.enabled) return 'disabled';
  if (!setup.csr) return 'notConnected';
  if (!setup.hasCertificate) return 'inProgress';
  return setup.certificateExpiresAt !== null && setup.certificateExpiresAt < now
    ? 'expired'
    : 'connected';
}

interface ARCAConnectionCardProps {
  setup: ARCASetup;
  now: string;
}

export function ARCAConnectionCard({ setup, now }: ARCAConnectionCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.settings.connection');
  const state = arcaConnectionState(setup, now);
  const installed = state === 'connected' || state === 'expired';

  return (
    <Card className="gap-y-4">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <CardTitle>{t('title')}</CardTitle>
          <Badge tone={BADGE_TONE[state]}>{t(`states.${state}`)}</Badge>
        </div>
        <CardDescription>{t(`bodies.${state}`)}</CardDescription>
      </CardHeader>
      {installed ? (
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-2">
            <div className="flex flex-col gap-y-0.5">
              <dt className="text-foreground-muted">{t('cuit')}</dt>
              <dd className="text-foreground tabular-nums">{formatCuit(setup.taxId)}</dd>
            </div>
            {setup.certificateExpiresAt ? (
              <div className="flex flex-col gap-y-0.5">
                <dt className="text-foreground-muted">{t('expires')}</dt>
                <dd
                  className={
                    state === 'expired'
                      ? 'text-danger-foreground tabular-nums'
                      : 'text-foreground tabular-nums'
                  }
                >
                  {fmt.date(setup.certificateExpiresAt)}
                </dd>
              </div>
            ) : null}
          </dl>
        </CardContent>
      ) : null}
      {state === 'disabled' ? null : (
        <CardFooter>
          <Button asChild variant={state === 'connected' ? 'outline' : 'default'}>
            <Link href={ROUTES.arcaSettings}>{t(`actions.${ACTION[state]}`)}</Link>
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}
