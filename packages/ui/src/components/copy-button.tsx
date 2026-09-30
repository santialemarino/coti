'use client';

import * as React from 'react';
import { CheckIcon, CopyIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Button } from './button';

/* How long the button reads "copied" before it offers to copy again. */
const COPIED_RESET_MS = 2000;

interface CopyButtonProps extends Omit<
  React.ComponentProps<typeof Button>,
  'children' | 'onClick'
> {
  value: string;
  labels: { copy: string; copied: string };
  /* The clipboard can refuse the write (permissions, an insecure origin); the app words that. */
  onCopyError?: (error: unknown) => void;
}

/*
 * Both icons and both labels are stacked in one grid cell and crossfaded, so the button never
 * changes width between "copy" and "copied". The confirmation is announced through a live region,
 * since the visible labels are hidden from assistive tech to keep the button's name stable.
 */
function CopyButton({
  value,
  labels,
  onCopyError,
  variant = 'outline',
  size = 'sm',
  className,
  ...props
}: CopyButtonProps) {
  const [copiedValue, setCopiedValue] = React.useState<string | null>(null);
  const timer = React.useRef(0);
  const copied = copiedValue === value;

  React.useEffect(() => () => window.clearTimeout(timer.current), []);

  async function handleClick() {
    window.clearTimeout(timer.current);
    try {
      await navigator.clipboard.writeText(value);
    } catch (error) {
      setCopiedValue(null);
      onCopyError?.(error);
      return;
    }
    setCopiedValue(value);
    timer.current = window.setTimeout(() => setCopiedValue(null), COPIED_RESET_MS);
  }

  const layer = 'col-start-1 row-start-1 transition-[opacity,scale] duration-150 ease-out-soft';

  return (
    <>
      <Button
        variant={variant}
        size={size}
        aria-label={labels.copy}
        onClick={handleClick}
        className={className}
        {...props}
      >
        <span aria-hidden="true" className="grid">
          <CopyIcon
            className={cn(
              layer,
              'size-3.5',
              copied ? 'scale-50 opacity-0' : 'scale-100 opacity-100',
            )}
          />
          <CheckIcon
            className={cn(
              layer,
              'size-3.5',
              copied ? 'scale-100 opacity-100' : 'scale-50 opacity-0',
            )}
          />
        </span>
        <span aria-hidden="true" className="grid text-left">
          <span className={cn(layer, copied ? 'opacity-0' : 'opacity-100')}>{labels.copy}</span>
          <span className={cn(layer, copied ? 'opacity-100' : 'opacity-0')}>{labels.copied}</span>
        </span>
      </Button>
      <span role="status" aria-live="polite" className="sr-only">
        {copied ? labels.copied : ''}
      </span>
    </>
  );
}

export { CopyButton };
