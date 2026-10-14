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
import type { ARCAConnectionState } from '@/lib/utils/arca-connection';

const CONNECTION_TONE: Record<ARCAConnectionState, 'neutral' | 'warning' | 'success' | 'danger'> = {
  disabled: 'neutral',
  notConnected: 'neutral',
  inProgress: 'warning',
  pointsMissing: 'warning',
  expiringSoon: 'warning',
  expired: 'danger',
  connected: 'success',
};

const ACTION: Record<Exclude<ARCAConnectionState, 'disabled'>, { label: string; href: string }> = {
  notConnected: { label: 'start', href: ROUTES.arcaSettings },
  inProgress: { label: 'continue', href: ROUTES.arcaSettings },
  pointsMissing: { label: 'continue', href: ROUTES.arcaStep('points-of-sale') },
  expiringSoon: { label: 'manage', href: ROUTES.arcaSettings },
  expired: { label: 'manage', href: ROUTES.arcaSettings },
  connected: { label: 'manage', href: ROUTES.arcaSettings },
};

// The states in which a certificate is installed, so the CUIT and its expiry are worth showing.
const INSTALLED: readonly ARCAConnectionState[] = [
  'connected',
  'pointsMissing',
  'expiringSoon',
  'expired',
];

interface ARCAConnectionCardProps {
  setup: ARCASetup;
  state: ARCAConnectionState;
}

export function ARCAConnectionCard({ setup, state }: ARCAConnectionCardProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.settings.connection');
  const expiresOn = setup.certificateExpiresAt ? fmt.date(setup.certificateExpiresAt) : '';

  return (
    <Card className="gap-y-4">
      <CardHeader>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <CardTitle>{t('title')}</CardTitle>
          <Badge tone={CONNECTION_TONE[state]}>{t(`states.${state}`)}</Badge>
        </div>
        <CardDescription>{t(`bodies.${state}`, { date: expiresOn })}</CardDescription>
      </CardHeader>
      {INSTALLED.includes(state) ? (
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-8 gap-y-3 text-paragraph-sm sm:grid-cols-2">
            <div className="flex flex-col gap-y-0.5">
              <dt className="text-foreground-muted">{t('cuit')}</dt>
              <dd className="text-foreground tabular-nums">{formatCuit(setup.taxId)}</dd>
            </div>
            {expiresOn ? (
              <div className="flex flex-col gap-y-0.5">
                <dt className="text-foreground-muted">{t('expires')}</dt>
                <dd
                  className={
                    state === 'expired'
                      ? 'text-danger-foreground tabular-nums'
                      : 'text-foreground tabular-nums'
                  }
                >
                  {expiresOn}
                </dd>
              </div>
            ) : null}
          </dl>
        </CardContent>
      ) : null}
      {state === 'disabled' ? null : (
        <CardFooter>
          <Button asChild variant={state === 'connected' ? 'outline' : 'default'}>
            <Link href={ACTION[state].href}>{t(`actions.${ACTION[state].label}`)}</Link>
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}
