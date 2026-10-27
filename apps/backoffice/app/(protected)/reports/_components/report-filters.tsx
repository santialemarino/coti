'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';

import { Button, Input, Label } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';

// Cross-field validation needs state; native `required` already covers one empty date.
export function ReportFilters({ from, to }: { from: string; to: string }) {
  const t = useTranslations('reports.filters');
  const [fromValue, setFromValue] = useState(from);
  const [toValue, setToValue] = useState(to);
  const [error, setError] = useState('');

  return (
    <form
      action={ROUTES.reports}
      method="get"
      className="flex flex-wrap items-end gap-4"
      onSubmit={(event) => {
        if (fromValue && toValue && fromValue > toValue) {
          event.preventDefault();
          setError(t('invalidRange'));
          return;
        }
        setError('');
      }}
    >
      <div className="flex flex-col gap-y-1.5">
        <Label htmlFor="report-from">{t('from')}</Label>
        <Input
          id="report-from"
          name="from"
          type="date"
          required
          containerClassName="w-44"
          value={fromValue}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? 'report-range-error' : undefined}
          onChange={(event) => {
            setFromValue(event.target.value);
            setError('');
          }}
        />
      </div>
      <div className="flex flex-col gap-y-1.5">
        <Label htmlFor="report-to">{t('to')}</Label>
        <Input
          id="report-to"
          name="to"
          type="date"
          required
          containerClassName="w-44"
          value={toValue}
          aria-invalid={Boolean(error)}
          aria-describedby={error ? 'report-range-error' : undefined}
          onChange={(event) => {
            setToValue(event.target.value);
            setError('');
          }}
        />
      </div>
      <div className="flex items-center gap-2">
        <Button type="submit">{t('apply')}</Button>
        <Button asChild variant="ghost">
          <Link href={ROUTES.reports}>{t('reset')}</Link>
        </Button>
      </div>
      {error ? (
        <p
          id="report-range-error"
          role="alert"
          className="w-full text-paragraph-xs text-danger-foreground"
        >
          {error}
        </p>
      ) : null}
    </form>
  );
}
