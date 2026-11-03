'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Building2Icon, RadioIcon, TagIcon, UsersIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Combobox, Input, Label, MultiCombobox } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';

const REPORT_STATUSES = [
  'DRAFT',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED',
] as const;

const CHANNELS = ['WHATSAPP', 'EMAIL', 'WEBAPP', 'MANUAL_ENTRY'] as const;

const ALL = 'all';

interface FilterOption {
  id: string;
  name: string;
}

interface ReportFiltersProps {
  from: string;
  to: string;
  statuses?: string[];
  channel?: string;
  sellerId?: string;
  clientId?: string;
  sellers?: FilterOption[];
  clients?: FilterOption[];
  showSeller?: boolean;
}

// Cross-field validation needs state; native `required` already covers one empty date.
export function ReportFilters({
  from,
  to,
  statuses = [],
  channel,
  sellerId,
  clientId,
  sellers = [],
  clients = [],
  showSeller = false,
}: ReportFiltersProps) {
  const t = useTranslations('reports.filters');
  const tStatus = useTranslations('rfqs.status');
  const tChannel = useTranslations('rfqs.channels');
  const [fromValue, setFromValue] = useState(from);
  const [toValue, setToValue] = useState(to);
  const [statusValues, setStatusValues] = useState(statuses);
  const [channelValue, setChannelValue] = useState(channel ?? ALL);
  const [sellerValue, setSellerValue] = useState(sellerId ?? ALL);
  const [clientValue, setClientValue] = useState(clientId ?? ALL);
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
      <MultiCombobox
        options={REPORT_STATUSES.map((status) => ({
          value: status,
          label: tStatus(status),
        }))}
        values={statusValues}
        onValuesChange={setStatusValues}
        placeholder={t('status')}
        summaryLabel={(count) => t('statusCount', { count })}
        clearLabel={t('clearStatus')}
        icon={<TagIcon aria-hidden="true" className="size-4" />}
        aria-label={t('status')}
        className="min-w-40 flex-1"
      />
      <Combobox
        options={[
          { value: ALL, label: t('allChannel') },
          ...CHANNELS.map((value) => ({
            value,
            label: tChannel(value.toLowerCase()),
          })),
        ]}
        value={channelValue}
        resetValue={ALL}
        onValueChange={setChannelValue}
        placeholder={t('channel')}
        icon={<RadioIcon aria-hidden="true" className="size-4" />}
        aria-label={t('channel')}
        className="min-w-36 flex-1"
      />
      {showSeller ? (
        <Combobox
          options={[
            { value: ALL, label: t('allSeller') },
            ...sellers.map((seller) => ({ value: seller.id, label: seller.name })),
          ]}
          value={sellerValue}
          resetValue={ALL}
          onValueChange={setSellerValue}
          placeholder={t('seller')}
          icon={<UsersIcon aria-hidden="true" className="size-4" />}
          aria-label={t('seller')}
          className="min-w-36 flex-1"
        />
      ) : null}
      <Combobox
        options={[
          { value: ALL, label: t('allClient') },
          ...clients.map((client) => ({ value: client.id, label: client.name })),
        ]}
        value={clientValue}
        resetValue={ALL}
        onValueChange={setClientValue}
        placeholder={t('client')}
        icon={<Building2Icon aria-hidden="true" className="size-4" />}
        aria-label={t('client')}
        className="min-w-36 flex-1"
      />
      {statusValues.map((status) => (
        <input key={status} type="hidden" name="status" value={status} />
      ))}
      {channelValue !== ALL ? <input type="hidden" name="channel" value={channelValue} /> : null}
      {showSeller && sellerValue !== ALL ? (
        <input type="hidden" name="seller_id" value={sellerValue} />
      ) : null}
      {clientValue !== ALL ? <input type="hidden" name="client_id" value={clientValue} /> : null}
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
