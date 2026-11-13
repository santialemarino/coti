'use client';

import { useState } from 'react';
import Form from 'next/form';
import Link from 'next/link';
import {
  Building2Icon,
  CircleDotIcon,
  RadioIcon,
  StoreIcon,
  TagIcon,
  UsersIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Combobox, DateRangePicker, Label, MultiCombobox } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { useDateRangeCopy } from '@/hooks/use-date-range-copy';
import { useNavigationPending } from '@/hooks/use-navigation-pending';
import { DATE_RANGE_PRESETS } from '@/lib/utils/date-ranges';

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
  branchIds?: string[];
  tagIds?: string[];
  sellers?: FilterOption[];
  clients?: FilterOption[];
  branches?: FilterOption[];
  tags?: FilterOption[];
  showSeller?: boolean;
  // Only a report that spans several branches can narrow by branch.
  showBranch?: boolean;
  // A trend bucket chosen in the chart rides along with the filters; absent, the server picks one.
  granularity?: string;
}

// A GET form through next/form: the URL carries the filters and navigation stays client-side; the
// old report stays on screen, with the filters as chosen, until the new one arrives.
export function ReportFilters({
  from,
  to,
  statuses = [],
  channel,
  sellerId,
  clientId,
  branchIds = [],
  tagIds = [],
  sellers = [],
  clients = [],
  branches = [],
  tags = [],
  showSeller = false,
  showBranch = false,
  granularity,
}: ReportFiltersProps) {
  const t = useTranslations('reports.filters');
  const tStatus = useTranslations('reports.statusLabels');
  const tChannel = useTranslations('rfqs.channels');
  const [period, setPeriod] = useState({ from, to });
  const [statusValues, setStatusValues] = useState(statuses);
  const [channelValue, setChannelValue] = useState(channel ?? ALL);
  const [sellerValue, setSellerValue] = useState(sellerId ?? ALL);
  const [clientValue, setClientValue] = useState(clientId ?? ALL);
  const [branchValues, setBranchValues] = useState(branchIds);
  const [tagValues, setTagValues] = useState(tagIds);
  const [error, setError] = useState('');
  const { begin } = useNavigationPending();
  const rangeCopy = useDateRangeCopy(DATE_RANGE_PRESETS);

  return (
    <Form
      action={ROUTES.reports}
      className="flex flex-wrap items-end gap-4"
      onSubmit={(event) => {
        if (period.from && period.to && period.from > period.to) {
          event.preventDefault();
          setError(t('invalidRange'));
          return;
        }
        setError('');
        // next/form navigates on its own; the shell is told, so the bar shows while the report loads.
        const query = new URLSearchParams(
          [...new FormData(event.currentTarget)].map(([key, value]) => [key, String(value)]),
        ).toString();
        begin(query ? `${ROUTES.reports}?${query}` : ROUTES.reports);
      }}
    >
      <div className="flex w-full flex-col gap-y-1.5 sm:w-auto">
        <Label htmlFor="report-period">{t('period')}</Label>
        <DateRangePicker
          id="report-period"
          value={period}
          onValueChange={(next) => {
            setPeriod(next);
            setError('');
          }}
          placeholder={t('period')}
          labels={rangeCopy.labels}
          presets={rangeCopy.presets}
          names={{ from: 'from', to: 'to' }}
          className="sm:w-72"
          aria-invalid={Boolean(error)}
          aria-describedby={error ? 'report-range-error' : undefined}
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
        icon={<CircleDotIcon aria-hidden="true" className="size-4" />}
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
      {tags.length > 0 ? (
        <MultiCombobox
          options={tags.map((tag) => ({ value: tag.id, label: tag.name }))}
          values={tagValues}
          onValuesChange={setTagValues}
          placeholder={t('tag')}
          summaryLabel={(count) => t('tagCount', { count })}
          clearLabel={t('clearTag')}
          icon={<TagIcon aria-hidden="true" className="size-4" />}
          aria-label={t('tag')}
          className="min-w-36 flex-1"
        />
      ) : null}
      {showBranch ? (
        <MultiCombobox
          options={branches.map((branch) => ({ value: branch.id, label: branch.name }))}
          values={branchValues}
          onValuesChange={setBranchValues}
          placeholder={t('branch')}
          summaryLabel={(count) => t('branchCount', { count })}
          clearLabel={t('clearBranch')}
          icon={<StoreIcon aria-hidden="true" className="size-4" />}
          aria-label={t('branch')}
          className="min-w-36 flex-1"
        />
      ) : null}
      {statusValues.map((status) => (
        <input key={status} type="hidden" name="status" value={status} />
      ))}
      {channelValue !== ALL ? <input type="hidden" name="channel" value={channelValue} /> : null}
      {showSeller && sellerValue !== ALL ? (
        <input type="hidden" name="seller_id" value={sellerValue} />
      ) : null}
      {clientValue !== ALL ? <input type="hidden" name="client_id" value={clientValue} /> : null}
      {tagValues.map((tag) => (
        <input key={tag} type="hidden" name="tag_id" value={tag} />
      ))}
      {granularity ? <input type="hidden" name="granularity" value={granularity} /> : null}
      {showBranch
        ? branchValues.map((branch) => (
            <input key={branch} type="hidden" name="branch_id" value={branch} />
          ))
        : null}
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
    </Form>
  );
}
