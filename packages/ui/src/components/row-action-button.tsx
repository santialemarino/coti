'use client';

import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Button } from './button';
import { Tooltip, TooltipContent, TooltipTrigger } from './tooltip';

interface RowActionButtonProps extends Omit<
  React.ComponentProps<typeof Button>,
  'children' | 'size'
> {
  icon: LucideIcon;
  /* Both the tooltip body and the accessible name — an icon-only control needs a real name. */
  label: string;
  tone?: 'default' | 'danger';
}

/*
 * The icon action in a table row. A tooltip is the only label it has, which is why the label prop is
 * required and doubles as the aria-label.
 *
 * Do not disable one of these to signal "not allowed": a Radix tooltip never fires on a disabled
 * trigger, so the explanation becomes unreachable exactly when it is needed. Hide the action and
 * explain the absence instead.
 */
const RowActionButton = React.forwardRef<HTMLButtonElement, RowActionButtonProps>(
  ({ icon: Icon, label, tone = 'default', className, ...props }, ref) => (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          ref={ref}
          variant="ghost"
          size="icon-sm"
          aria-label={label}
          className={cn(
            'text-foreground-subtle',
            tone === 'danger'
              ? 'hover:bg-danger-subtle hover:text-danger-foreground'
              : 'hover:text-foreground',
            className,
          )}
          {...props}
        >
          <Icon aria-hidden="true" className="size-4" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  ),
);

RowActionButton.displayName = 'RowActionButton';

export { RowActionButton };
