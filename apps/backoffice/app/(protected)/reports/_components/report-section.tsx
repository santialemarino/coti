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
  className?: string;
  children?: ReactNode;
}

// One report card: its title, an optional description, and either its body or its empty line.
export function ReportSection({
  title,
  description,
  empty,
  flush = false,
  className,
  children,
}: ReportSectionProps) {
  const emptyState = empty ? (
    <EmptyState icon={ChartColumnIcon} size="inline" title={empty} />
  ) : null;

  if (flush) {
    return (
      <Card className={cn('gap-y-0 overflow-hidden py-0', className)}>
        <CardHeader className="pb-4 pt-5">
          <CardTitle>{title}</CardTitle>
          {description ? <CardDescription>{description}</CardDescription> : null}
        </CardHeader>
        {emptyState ? <CardContent className="pb-5">{emptyState}</CardContent> : children}
      </Card>
    );
  }

  return (
    <Card className={cn('gap-y-4 py-5', className)}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>{emptyState ?? children}</CardContent>
    </Card>
  );
}
