'use client';

import type { ComponentProps } from 'react';
import { useTranslations } from 'next-intl';

import { Badge } from '@repo/ui/components';
import type { RfqStatus } from '@/lib/api/rfqs';

// Status labels are i18n; the tone is what picks the surface that shows them. One mapping for the
// sidebar chips, the table badge and the filter tabs, so a state always looks the same everywhere.
export const STATUS_TONE: Record<RfqStatus, ComponentProps<typeof Badge>['tone']> = {
  RECEIVED: 'neutral',
  GENERATED: 'brand',
  QUOTED: 'brand',
  SENT: 'outline',
  CHANGE_REQUESTED: 'warning',
  ACCEPTED: 'success',
  REJECTED: 'danger',
};

// The order the domain documents them in, used by the tabs and by sorting.
export const STATUS_ORDER: readonly RfqStatus[] = [
  'RECEIVED',
  'GENERATED',
  'QUOTED',
  'SENT',
  'CHANGE_REQUESTED',
  'ACCEPTED',
  'REJECTED',
];

interface RfqStatusBadgeProps {
  status: RfqStatus;
  size?: 'sm' | 'default';
}

export function RfqStatusBadge({ status, size }: RfqStatusBadgeProps) {
  const t = useTranslations('rfqs');
  return (
    <Badge tone={STATUS_TONE[status]} size={size} dot>
      {t(`status.${status}`)}
    </Badge>
  );
}
