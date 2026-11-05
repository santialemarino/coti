'use client';

import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { ToggleGroup, ToggleGroupItem } from '@repo/ui/components';
import type { ReportGranularity } from '@/lib/api/reports';

const GRANULARITIES: ReportGranularity[] = ['day', 'week', 'month'];

// The trend's bucket size; it lives in the URL so a shared link and the PDF read the same buckets.
export function ReportGranularityToggle({ value }: { value: ReportGranularity }) {
  const t = useTranslations('reports.timeline.granularity');
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  return (
    <ToggleGroup
      type="single"
      variant="segmented"
      size="sm"
      value={value}
      aria-label={t('label')}
      onValueChange={(next) => {
        if (!next || next === value) return;
        const params = new URLSearchParams(searchParams.toString());
        params.set('granularity', next);
        router.replace(`${pathname}?${params.toString()}`, { scroll: false });
      }}
    >
      {GRANULARITIES.map((granularity) => (
        <ToggleGroupItem key={granularity} value={granularity}>
          {t(granularity)}
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  );
}
