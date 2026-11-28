'use client';

import Link from 'next/link';
import { useTranslations } from 'next-intl';

import {
  Badge,
  Button,
  Callout,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Progress,
} from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import type { OperationalReadiness } from '@/lib/api/onboarding';

export function OperationalReadinessCard({ readiness }: { readiness: OperationalReadiness }) {
  const t = useTranslations('onboarding.readiness');
  const checks = [
    readiness.accountReady,
    readiness.branches.length > 0,
    ...readiness.branches.flatMap((branch) => [
      branch.products > 0,
      branch.products > 0 && branch.missingPrices === 0,
      branch.hasStaff,
    ]),
  ];
  const done = checks.filter(Boolean).length;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-paragraph-sm text-foreground-muted">{t('description')}</p>
        <Progress
          value={(done / checks.length) * 100}
          label={t('progress', { done, total: checks.length })}
        />
        <p className="text-paragraph-sm">{t('progress', { done, total: checks.length })}</p>
      </CardHeader>
      <CardContent className="flex flex-col gap-y-4">
        {!readiness.accountReady ? (
          <Callout
            tone="warning"
            title={t('account')}
            action={
              <Button asChild variant="outline">
                <Link href={ROUTES.accountSettings}>{t('resolve')}</Link>
              </Button>
            }
          />
        ) : null}
        {readiness.branches.length === 0 ? (
          <Callout
            tone="warning"
            title={t('noBranches')}
            action={
              <Button asChild variant="outline">
                <Link href={ROUTES.branchSettings}>{t('resolve')}</Link>
              </Button>
            }
          />
        ) : null}
        {readiness.branches.map((branch) => (
          <section key={branch.id} className="flex flex-col gap-y-3">
            <h2 className="text-heading-6">{branch.name}</h2>
            <ul className="flex flex-col gap-y-2">
              {[
                {
                  key: 'catalog',
                  done: branch.products > 0,
                  href: ROUTES.branchCatalogSettings(branch.id),
                },
                {
                  key: 'prices',
                  done: branch.products > 0 && branch.missingPrices === 0,
                  href: ROUTES.branchPriceSettings(branch.id),
                },
                { key: 'staff', done: branch.hasStaff, href: ROUTES.userSettings },
                {
                  key: 'channels',
                  done: branch.hasChannel,
                  href: ROUTES.branchChannels(branch.id),
                  optional: true,
                },
              ].map((step) => (
                <li
                  key={step.key}
                  className="flex flex-wrap items-center justify-between p-3 gap-2 border border-border rounded-lg"
                >
                  <span className="text-paragraph-sm">{t(step.key)}</span>
                  <div className="flex items-center gap-x-2">
                    <Badge tone={step.done ? 'success' : step.optional ? 'neutral' : 'warning'}>
                      {t(step.done ? 'complete' : step.optional ? 'optional' : 'pending')}
                    </Badge>
                    {!step.done ? (
                      <Button asChild variant="outline" size="sm">
                        <Link href={step.href}>{t('resolve')}</Link>
                      </Button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
            {branch.missingPrices > 0 ? (
              <Callout
                tone="warning"
                title={t('priceAlert', { count: branch.missingPrices })}
                action={
                  <Button asChild variant="outline">
                    <Link href={ROUTES.branchPriceSettings(branch.id)}>{t('managePrices')}</Link>
                  </Button>
                }
              >
                {t('priceScope', { branch: branch.name })}
              </Callout>
            ) : null}
          </section>
        ))}
      </CardContent>
    </Card>
  );
}
