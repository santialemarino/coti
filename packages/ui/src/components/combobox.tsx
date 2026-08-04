'use client';

import * as React from 'react';
import { CheckIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Button } from './button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from './command';
import { DropdownChevron } from './dropdown-chevron';
import { Popover, PopoverContent, PopoverTrigger } from './popover';

/* How long a blind type-ahead buffer survives between keystrokes, matching native select behaviour. */
const TYPEAHEAD_RESET_MS = 700;

export interface ComboboxOption {
  value: string;
  label: string;
  icon?: React.ReactNode;
  disabled?: boolean;
  /* Options sharing a group render under one heading, in first-seen order. */
  group?: string;
}

interface ComboboxProps {
  options: ComboboxOption[];
  value: string | null;
  onValueChange: (value: string) => void;
  placeholder: string;
  /* Shown when a search yields nothing. Required whenever `searchable`. */
  emptyLabel?: string;
  searchPlaceholder?: string;
  /*
   * A search box only earns its place once the list is long enough to scan poorly. Below that it
   * costs a click and a focus trap, so short lists behave like a native select: blind type-ahead
   * plus arrow keys.
   */
  searchable?: boolean;
  disabled?: boolean;
  id?: string;
  className?: string;
  contentClassName?: string;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
}

/* Accent-insensitive, so typing "arena" still reaches "Árena" and vice versa. */
function fold(value: string) {
  return value
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase();
}

/*
 * The single dropdown in the design system. It is a Popover rather than a Radix Select because
 * Radix Select has no exit presence — it animates open and then snaps shut — and a control whose
 * close looks worse than its open is a control people notice.
 */
function Combobox({
  options,
  value,
  onValueChange,
  placeholder,
  emptyLabel,
  searchPlaceholder,
  searchable = false,
  disabled = false,
  id,
  className,
  contentClassName,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
}: ComboboxProps) {
  const [open, setOpen] = React.useState(false);
  /* The highlighted option. Controlled so type-ahead can move it without a search query. */
  const [highlighted, setHighlighted] = React.useState<string>('');
  const typeahead = React.useRef({ buffer: '', timer: 0 });
  const listRef = React.useRef<HTMLDivElement>(null);

  const selected = options.find((option) => option.value === value) ?? null;

  const groups = React.useMemo(() => {
    const byGroup = new Map<string, ComboboxOption[]>();
    options.forEach((option) => {
      const key = option.group ?? '';
      const bucket = byGroup.get(key);
      if (bucket) bucket.push(option);
      else byGroup.set(key, [option]);
    });
    return [...byGroup.entries()];
  }, [options]);

  React.useEffect(() => {
    if (open) setHighlighted(value ?? options[0]?.value ?? '');
  }, [open, value, options]);

  React.useEffect(() => () => window.clearTimeout(typeahead.current.timer), []);

  /* Blind type-ahead for the non-searchable list: jump the highlight, never filter. */
  function handleTypeahead(event: React.KeyboardEvent) {
    if (searchable) return;
    if (event.key.length !== 1 || event.metaKey || event.ctrlKey || event.altKey) return;

    const state = typeahead.current;
    window.clearTimeout(state.timer);
    state.buffer += event.key;
    state.timer = window.setTimeout(() => {
      state.buffer = '';
    }, TYPEAHEAD_RESET_MS);

    const needle = fold(state.buffer);
    const match =
      options.find((option) => !option.disabled && fold(option.label).startsWith(needle)) ??
      options.find((option) => !option.disabled && fold(option.label).includes(needle));
    if (match) setHighlighted(match.value);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-invalid={ariaInvalid}
          aria-describedby={ariaDescribedBy}
          disabled={disabled}
          className={cn(
            /* The trigger reads as a field, so it swaps Button's medium size token for the input one
               at the same size — a bare `font-normal` would leave both weights in the class list. */
            'w-full justify-between text-paragraph-sm',
            !selected && 'text-foreground-subtle',
            ariaInvalid && 'border-danger focus-visible:border-danger focus-visible:ring-danger/30',
            className,
          )}
        >
          <span className="flex min-w-0 items-center gap-x-2">
            {selected?.icon}
            <span className="truncate">{selected?.label ?? placeholder}</span>
          </span>
          <DropdownChevron open={open} />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        /*
         * Without a search box there is no focusable descendant, so Radix parks focus on the popover
         * itself and cmdk — which only sees keys that originate inside its own root — never receives
         * an arrow key. Focusing the list instead puts the caret inside `Command`.
         */
        onOpenAutoFocus={
          searchable
            ? undefined
            : (event) => {
                event.preventDefault();
                listRef.current?.focus();
              }
        }
        className={cn('w-(--radix-popover-trigger-width) p-0', contentClassName)}
      >
        <Command
          value={highlighted}
          onValueChange={setHighlighted}
          /* Filtering is cmdk's job only when there is a query to filter by. Without a search box
             the list must never resize, or the popover re-flips mid-interaction. */
          shouldFilter={searchable}
          onKeyDown={handleTypeahead}
        >
          {searchable ? <CommandInput placeholder={searchPlaceholder} /> : null}
          <CommandList ref={listRef}>
            {searchable ? <CommandEmpty>{emptyLabel}</CommandEmpty> : null}
            {groups.map(([group, groupOptions]) => (
              <CommandGroup key={group || 'ungrouped'} heading={group || undefined}>
                {groupOptions.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={[option.label]}
                    disabled={option.disabled}
                    onSelect={() => {
                      onValueChange(option.value);
                      setOpen(false);
                    }}
                  >
                    {option.icon}
                    <span className="truncate">{option.label}</span>
                    <CheckIcon
                      aria-hidden="true"
                      className={cn(
                        'ml-auto size-4 shrink-0 transition-[opacity,scale] duration-150 ease-out-soft',
                        option.value === value ? 'scale-100 opacity-100' : 'scale-50 opacity-0',
                      )}
                    />
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export { Combobox };
