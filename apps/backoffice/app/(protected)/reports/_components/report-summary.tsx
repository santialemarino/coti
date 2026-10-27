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
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  detail?: string;
  className?: string;
}) {
  return (
    <Card className={cn('gap-y-3 py-5', className)}>
      <div className="flex items-start justify-between gap-x-3 px-5">
        <p className="text-paragraph-sm text-foreground-muted">{label}</p>
        <span className="grid size-8 shrink-0 place-items-center rounded-full bg-accent text-accent-foreground">
          <Icon aria-hidden="true" className="size-4" />
        </span>
      </div>
      <div className="px-5">
        <p className="text-heading-4 tabular-nums text-foreground">{value}</p>
        {detail ? <p className="mt-1 text-paragraph-xs text-foreground-subtle">{detail}</p> : null}
      </div>
    </Card>
  );
}
