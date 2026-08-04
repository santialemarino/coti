'use client';

import * as React from 'react';
import { SearchIcon, XIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Input } from './input';

interface SearchInputProps extends Omit<
  React.ComponentProps<typeof Input>,
  'endIcon' | 'startIcon'
> {
  /* Omit to render a search field with no clear affordance. */
  onClear?: () => void;
  clearLabel?: string;
}

/*
 * The clear button stays mounted and fades out rather than unmounting, so the field's text never
 * reflows as the button appears. It is also removed from the tab order while hidden — a control the
 * user cannot see should not be a tab stop.
 */
const SearchInput = React.forwardRef<HTMLInputElement, SearchInputProps>(
  ({ className, containerClassName, onClear, clearLabel, ...props }, ref) => {
    const hasValue = Boolean(props.value);

    return (
      <Input
        ref={ref}
        type="search"
        startIcon={<SearchIcon aria-hidden="true" className="size-4" />}
        endIcon={
          onClear ? (
            <button
              type="button"
              aria-label={clearLabel}
              tabIndex={hasValue ? 0 : -1}
              onClick={onClear}
              className={cn(
                'group/clear flex p-0.5 rounded-md outline-none',
                'transition-[color,opacity,scale] duration-150 ease-out-soft',
                'text-foreground-subtle hover:text-foreground focus-visible:text-foreground',
                hasValue ? 'scale-100 opacity-100' : 'pointer-events-none scale-75 opacity-0',
              )}
            >
              {/* A held 1.1, not a one-shot bump — the button's own scale already drives its
                  appearance, and a keyframe returning to rest would fight it. */}
              <XIcon
                aria-hidden="true"
                className={cn(
                  'size-3.5 transition-[scale] duration-150 ease-out-soft',
                  'group-hover/clear:scale-110 group-focus-visible/clear:scale-110',
                )}
              />
            </button>
          ) : undefined
        }
        className={cn('[&::-webkit-search-cancel-button]:hidden', className)}
        containerClassName={cn('min-w-48', containerClassName)}
        {...props}
      />
    );
  },
);

SearchInput.displayName = 'SearchInput';

export { SearchInput };
