import type { ReactNode } from 'react';
import { ChartColumnIcon } from 'lucide-react';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  EmptyState,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';

interface ReportSectionProps {
  title: string;
  description?: string;
  // Replaces the body when the section has nothing to show.
  empty?: string;
  // Lists and tables run edge to edge; charts sit inside the card's padding.
  flush?: boolean;
  // A control beside the title that changes how the body reads, like the trend's granularity.
  action?: ReactNode;
  className?: string;
  children?: ReactNode;
}

// One report card: its title, an optional description, and either its body or its empty line.
export function ReportSection({
  title,
  description,
  empty,
  flush = false,
  action,
  className,
  children,
}: ReportSectionProps) {
  const heading = (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex min-w-0 flex-col gap-y-1.5">
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </div>
      {action}
    </div>
  );
  const emptyState = empty ? (
    <EmptyState icon={ChartColumnIcon} size="inline" title={empty} />
  ) : null;

  if (flush) {
    return (
      <Card className={cn('gap-y-0 overflow-hidden py-0', className)}>
        <CardHeader className="pb-4 pt-5">{heading}</CardHeader>
        {emptyState ? <CardContent className="pb-5">{emptyState}</CardContent> : children}
      </Card>
    );
  }

  return (
    <Card className={cn('gap-y-4 py-5', className)}>
      <CardHeader>{heading}</CardHeader>
      <CardContent>{emptyState ?? children}</CardContent>
    </Card>
  );
}
