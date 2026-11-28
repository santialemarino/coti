import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

import { Card } from '@repo/ui/components';
import { cn } from '@repo/ui/lib';

// One headline number with the single detail that keeps it from being misread.
export function SummaryCard({
  icon: Icon,
  label,
  value,
  detail,
  className,
  comparison,
}: {
  icon: LucideIcon;
  label: string;
  value: ReactNode;
  detail?: string;
  className?: string;
  comparison?: ReactNode;
}) {
  return (
    <Card className={cn('h-full min-w-0 gap-y-0 overflow-hidden py-0', className)}>
      <div className="flex items-center justify-between gap-x-3 px-5 pt-5">
        <p className="text-paragraph-sm-medium text-foreground-muted">{label}</p>
        <span className="grid size-8 shrink-0 place-items-center rounded-lg bg-accent text-accent-foreground">
          <Icon aria-hidden="true" className="size-4" />
        </span>
      </div>
      <div className="flex flex-1 flex-col px-5 pt-3 pb-5 gap-y-2">
        <div className="break-words text-heading-3 tabular-nums text-foreground">{value}</div>
        {detail ? <p className="text-paragraph-xs text-foreground-muted">{detail}</p> : null}
      </div>
      {comparison ? (
        <div className="mt-auto border-t border-border bg-sunken px-5 py-3">{comparison}</div>
      ) : null}
    </Card>
  );
}
