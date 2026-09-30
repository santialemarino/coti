'use client';

import { BellRingIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Badge, Tooltip, TooltipContent, TooltipTrigger } from '@repo/ui/components';
import { useFormatters } from '@/lib/i18n/formatters';

// The two stalls follow-up chases: the seller has not sent the quote, or the client has not answered.
const FOLLOWUP_STATUSES = new Set(['GENERATED', 'QUOTED', 'SENT']);

// Whether a flag still means something: nothing clears it once the quote moves on or is archived.
export function followupDue(flagged: boolean, status: string, archived: boolean): boolean {
  return flagged && !archived && FOLLOWUP_STATUSES.has(status);
}

interface FollowupBadgeProps {
  flaggedAt: string | null;
  size?: 'sm' | 'default';
  // A bell alone, for a dense row where the word would cost a column.
  compact?: boolean;
}

// Marks a quote the seller has to chase, in the follow-up colour and never by the tint alone.
export function FollowupBadge({
  flaggedAt,
  size = 'default',
  compact = false,
}: FollowupBadgeProps) {
  const fmt = useFormatters();
  const t = useTranslations('rfqs.followup');

  const description = flaggedAt
    ? t('since', { date: fmt.dateNumeric(flaggedAt) })
    : t('description');

  if (compact) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="grid size-5 shrink-0 place-items-center border border-followup-border bg-followup-subtle rounded-md text-followup-foreground">
            <BellRingIcon aria-hidden="true" className="size-3" />
            <span className="sr-only">{description}</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>{description}</TooltipContent>
      </Tooltip>
    );
  }

  return (
    <Badge tone="followup" size={size} title={description}>
      <BellRingIcon aria-hidden="true" />
      {t('badge')}
    </Badge>
  );
}
