import { useMemo } from 'react';
import { useTranslations } from 'next-intl';

import type { DateRangePickerLabels, DateRangePreset } from '@repo/ui/components';
import { dateRangePreset, type DateRangePresetKey } from '@/lib/utils/date-ranges';

const NO_PRESETS: readonly DateRangePresetKey[] = [];

// The words every range picker in the app shares, and the presets a screen asks for.
export function useDateRangeCopy(presetKeys: readonly DateRangePresetKey[] = NO_PRESETS) {
  const t = useTranslations('common.dateRange');

  const labels: DateRangePickerLabels = {
    from: t('from'),
    to: t('to'),
    pick: t('pick'),
    apply: t('apply'),
    cancel: t('cancel'),
    clearFrom: t('clearFrom'),
    clearTo: t('clearTo'),
    presets: t('presetsLabel'),
  };
  const presets = useMemo<DateRangePreset[]>(() => {
    const today = new Date();
    return presetKeys.map((key) => ({
      label: t(`presets.${key}`),
      value: dateRangePreset(key, today),
    }));
  }, [presetKeys, t]);

  return { labels, presets };
}
