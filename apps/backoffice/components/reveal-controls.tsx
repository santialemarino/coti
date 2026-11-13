'use client';

import { useTranslations } from 'next-intl';

import { ShowMore } from '@repo/ui/components';
import type { ProgressiveList } from '@repo/ui/hooks';

// The worded end of a list revealed in steps, for the import previews.
export function RevealControls({ list }: { list: ProgressiveList }) {
  const t = useTranslations('common.reveal');
  return (
    <ShowMore
      visible={list.visible}
      total={list.total}
      step={list.step}
      onShowMore={list.showMore}
      onShowAll={list.showAll}
      labels={{
        status: t('status', { visible: list.visible, total: list.total }),
        more: t('more', { count: Math.min(list.step, list.total - list.visible) }),
        all: t('all', { total: list.total }),
      }}
    />
  );
}
