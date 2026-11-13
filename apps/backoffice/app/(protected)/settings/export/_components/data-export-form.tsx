'use client';

import { useId, useState } from 'react';
import { DownloadIcon, StoreIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Card,
  CardContent,
  Collapsible,
  CollapsibleContent,
  Combobox,
  DateRangePicker,
  Hint,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
  Separator,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { useDateRangeCopy } from '@/hooks/use-date-range-copy';
import { errorCodeOf, throwApiError } from '@/lib/api/errors';
import { DATE_RANGE_PRESETS } from '@/lib/utils/date-ranges';

const ENTITIES = ['quotes', 'catalog', 'clients', 'invoices'] as const;
const FORMATS = ['csv', 'xml'] as const;
const ALL_BRANCHES = 'all';

type Entity = (typeof ENTITIES)[number];
type Format = (typeof FORMATS)[number];

interface ExportBranch {
  id: string;
  name: string;
  isActive: boolean;
}

interface DataExportFormProps {
  branches: ExportBranch[];
  // Invoices are offered only to an account that invoices or has invoiced.
  offersInvoices: boolean;
}

function filenameFrom(disposition: string | null, fallback: string): string {
  const match = disposition?.match(/filename="?([^";]+)"?/i);
  return match?.[1] ?? fallback;
}

export function DataExportForm({ branches, offersInvoices }: DataExportFormProps) {
  const t = useTranslations('dataExport');
  const errorMessage = useApiErrorMessage('dataExport');
  const fieldId = useId();
  const [entity, setEntity] = useState<Entity>('quotes');
  const [format, setFormat] = useState<Format>('csv');
  const [branchId, setBranchId] = useState(ALL_BRANCHES);
  const [period, setPeriod] = useState({ from: '', to: '' });
  const [rangeError, setRangeError] = useState('');
  const [pending, setPending] = useState(false);
  const rangeCopy = useDateRangeCopy(DATE_RANGE_PRESETS);

  const entities = ENTITIES.filter((value) => value !== 'invoices' || offersInvoices);
  // A branch is a choice only between several, and clients belong to the whole account.
  const showsBranch = branches.length > 1 && entity !== 'clients';
  const showsPeriod = entity === 'quotes' || entity === 'invoices';

  async function download() {
    if (showsPeriod && period.from && period.to && period.from > period.to) {
      setRangeError(t('period.invalidRange'));
      return;
    }
    setPending(true);
    try {
      const query = new URLSearchParams({ format });
      if (showsBranch && branchId !== ALL_BRANCHES) query.set('branch_id', branchId);
      if (showsPeriod && period.from) query.set('from', period.from);
      if (showsPeriod && period.to) query.set('to', period.to);
      const response = await fetch(`/api/exports/${entity}?${query.toString()}`, {
        cache: 'no-store',
      });
      if (!response.ok) await throwApiError(response);

      const filename = filenameFrom(
        response.headers.get('Content-Disposition'),
        `${entity}.${format}`,
      );
      const url = URL.createObjectURL(await response.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = filename;
      document.body.append(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
      toast.success(t('download.done', { filename }));
    } catch (error) {
      const code = errorCodeOf(error);
      toast.error(code === 'INTERNAL' ? t('download.error') : errorMessage(code));
    } finally {
      setPending(false);
    }
  }

  return (
    <Card className="max-w-3xl py-0">
      <CardContent className="flex flex-col py-6 gap-y-6">
        <fieldset className="flex flex-col gap-y-3">
          <legend className="pb-3 text-paragraph-sm-medium text-foreground">
            {t('entity.label')}
          </legend>
          <RadioGroup
            className="gap-y-3"
            value={entity}
            onValueChange={(value) => setEntity(value as Entity)}
          >
            {entities.map((value) => (
              <div key={value} className="flex items-start gap-x-2.5">
                <RadioGroupItem id={`${fieldId}-${value}`} value={value} className="mt-0.5" />
                <div className="flex flex-col gap-y-0.5">
                  <Label htmlFor={`${fieldId}-${value}`} className="cursor-pointer">
                    {t(`entity.${value}.label`)}
                  </Label>
                  <Hint>{t(`entity.${value}.hint`)}</Hint>
                </div>
              </div>
            ))}
          </RadioGroup>
        </fieldset>

        <Separator />

        <div className="flex flex-col gap-y-2">
          <span id={`${fieldId}-format`} className="text-paragraph-sm-medium text-foreground">
            {t('format.label')}
          </span>
          <ToggleGroup
            type="single"
            variant="segmented"
            size="sm"
            className="w-fit"
            value={format}
            aria-labelledby={`${fieldId}-format`}
            onValueChange={(value) => {
              if (value) setFormat(value as Format);
            }}
          >
            {FORMATS.map((value) => (
              <ToggleGroupItem key={value} value={value}>
                {t(`format.${value}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <Hint>{t(format === 'csv' ? 'format.csvHint' : 'format.xmlHint')}</Hint>
        </div>

        {/* The negative margin cancels the column's gap, so a hidden section leaves no band. */}
        <Collapsible open={showsBranch} className="-mt-6">
          <CollapsibleContent>
            <div className="flex flex-col pt-6 gap-y-2">
              <span className="text-paragraph-sm-medium text-foreground">{t('branch.label')}</span>
              <Combobox
                options={[
                  { value: ALL_BRANCHES, label: t('branch.all') },
                  ...branches.map((branch) => ({
                    value: branch.id,
                    label: branch.isActive
                      ? branch.name
                      : t('branch.closed', { name: branch.name }),
                  })),
                ]}
                value={branchId}
                resetValue={ALL_BRANCHES}
                onValueChange={setBranchId}
                placeholder={t('branch.label')}
                icon={<StoreIcon aria-hidden="true" className="size-4" />}
                aria-label={t('branch.label')}
                className="w-full sm:w-72"
              />
            </div>
          </CollapsibleContent>
        </Collapsible>

        <Collapsible open={showsPeriod} className="-mt-6">
          <CollapsibleContent>
            <div className="flex flex-col pt-6 gap-y-2">
              <Label htmlFor={`${fieldId}-period`}>{t('period.label')}</Label>
              <DateRangePicker
                id={`${fieldId}-period`}
                value={period}
                onValueChange={(next) => {
                  setPeriod(next);
                  setRangeError('');
                }}
                placeholder={t('period.placeholder')}
                labels={rangeCopy.labels}
                presets={rangeCopy.presets}
                openStart={t('period.openStart')}
                openEnd={t('period.openEnd')}
                className="sm:w-80"
                aria-invalid={Boolean(rangeError)}
                aria-describedby={`${fieldId}-period-hint`}
              />
              <p
                id={`${fieldId}-period-hint`}
                aria-live="polite"
                className={
                  rangeError
                    ? 'text-paragraph-sm text-danger-foreground'
                    : 'text-paragraph-sm text-foreground-muted'
                }
              >
                {rangeError || t(entity === 'quotes' ? 'period.quotesHint' : 'period.invoicesHint')}
              </p>
            </div>
          </CollapsibleContent>
        </Collapsible>

        <div className="flex">
          <PendingButton
            type="button"
            className="w-full sm:w-auto"
            pending={pending}
            pendingLabel={t('download.pending')}
            onClick={download}
          >
            <DownloadIcon aria-hidden="true" />
            {t('download.label')}
          </PendingButton>
        </div>
      </CardContent>
    </Card>
  );
}
