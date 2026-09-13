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
  CommandSeparator,
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

interface ComboboxSharedProps {
  options: ComboboxOption[];
  /* Shown on the trigger while nothing is chosen, in the subtle colour a placeholder uses. */
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
  /* Marks the dimension a filter narrows, so an unset trigger still says what it controls. */
  icon?: React.ReactNode;
  disabled?: boolean;
  id?: string;
  className?: string;
  contentClassName?: string;
  /* The trigger names itself after the current selection, so a standalone one needs this. */
  'aria-label'?: string;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
}

interface ComboboxProps extends ComboboxSharedProps {
  value: string | null;
  onValueChange: (value: string) => void;
  /*
   * The option that means "no filter". While it is selected the trigger reads as unset rather than
   * showing that option's label — otherwise a filter whose reset reads "Canales" is indistinguishable
   * from one actually narrowed to something, and the control looks like it is doing work it is not.
   */
  resetValue?: string;
}

/* Accent-insensitive, so typing "arena" still reaches "Árena" and vice versa. */
function fold(value: string) {
  return value
    .normalize('NFD')
    .replace(/\p{Diacritic}/gu, '')
    .toLowerCase();
}

function groupOptions(options: ComboboxOption[]): [string, ComboboxOption[]][] {
  const byGroup = new Map<string, ComboboxOption[]>();
  options.forEach((option) => {
    const key = option.group ?? '';
    const bucket = byGroup.get(key);
    if (bucket) bucket.push(option);
    else byGroup.set(key, [option]);
  });
  return [...byGroup.entries()];
}

/*
 * Radix publishes the trigger's width as a CSS variable only after a ResizeObserver has measured it,
 * which is a frame after the panel's first paint. A panel sized from that variable therefore opens at
 * its content width and snaps to the trigger's on the next frame — invisible on a quick click, and
 * impossible to miss when the pointer is held down on the trigger. Measuring the trigger in the same
 * event that opens the panel puts the real width in the first render instead.
 */
function useTriggerWidth() {
  const ref = React.useRef<HTMLButtonElement>(null);
  const [width, setWidth] = React.useState<number>();
  return {
    ref,
    style: width ? ({ width } as React.CSSProperties) : undefined,
    measure: () => setWidth(ref.current?.getBoundingClientRect().width),
  };
}

interface ComboboxTriggerProps extends React.ComponentProps<typeof Button> {
  open: boolean;
  /* True while nothing is chosen: the trigger shows the placeholder, muted. */
  unset: boolean;
  invalid?: boolean;
  label: React.ReactNode;
  icon?: React.ReactNode;
}

const ComboboxTrigger = React.forwardRef<HTMLButtonElement, ComboboxTriggerProps>(
  ({ open, unset, invalid, label, icon, className, ...props }, ref) => (
    <Button
      ref={ref}
      type="button"
      variant="outline"
      role="combobox"
      aria-expanded={open}
      aria-invalid={invalid}
      className={cn(
        /* The trigger reads as a field, so it swaps Button's medium size token for the input one
           at the same size — a bare `font-normal` would leave both weights in the class list. */
        'w-full justify-between text-paragraph-sm',
        unset && 'text-foreground-subtle',
        invalid && 'border-danger focus-visible:border-danger focus-visible:ring-danger/30',
        className,
      )}
      {...props}
    >
      <span className="flex min-w-0 items-center gap-x-2">
        {icon}
        <span className="truncate">{label}</span>
      </span>
      <DropdownChevron open={open} />
    </Button>
  ),
);

ComboboxTrigger.displayName = 'ComboboxTrigger';

/* Ticks the chosen option. Stacked in place and crossfaded, so the row never reflows. */
function SelectedMark({ selected }: { selected: boolean }) {
  return (
    <CheckIcon
      aria-hidden="true"
      className={cn(
        'ml-auto size-4 shrink-0 transition-[opacity,scale] duration-150 ease-out-soft',
        selected ? 'scale-100 opacity-100' : 'scale-50 opacity-0',
      )}
    />
  );
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
  resetValue,
  placeholder,
  emptyLabel,
  searchPlaceholder,
  searchable = false,
  icon,
  disabled = false,
  id,
  className,
  contentClassName,
  'aria-label': ariaLabel,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
}: ComboboxProps) {
  const [open, setOpen] = React.useState(false);
  /* The highlighted option. Controlled so type-ahead can move it without a search query. */
  const [highlighted, setHighlighted] = React.useState<string>('');
  const typeahead = React.useRef({ buffer: '', timer: 0 });
  const listRef = React.useRef<HTMLDivElement>(null);
  const trigger = useTriggerWidth();

  const selected = options.find((option) => option.value === value) ?? null;
  const unset = selected === null || selected.value === resetValue;
  const groups = React.useMemo(() => groupOptions(options), [options]);

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
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next) trigger.measure();
        setOpen(next);
      }}
    >
      <PopoverTrigger asChild>
        <ComboboxTrigger
          ref={trigger.ref}
          id={id}
          open={open}
          unset={unset}
          invalid={ariaInvalid}
          icon={unset ? icon : (selected?.icon ?? icon)}
          label={unset ? placeholder : (selected?.label ?? placeholder)}
          aria-label={ariaLabel}
          aria-describedby={ariaDescribedBy}
          disabled={disabled}
          className={className}
        />
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
        style={trigger.style}
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
            {groups.map(([group, groupItems]) => (
              <CommandGroup key={group || 'ungrouped'} heading={group || undefined}>
                {groupItems.map((option) => (
                  <React.Fragment key={option.value}>
                    <CommandItem
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
                      <SelectedMark selected={option.value === value} />
                    </CommandItem>
                    {/* The reset is an escape from the list, not a peer of its entries. */}
                    {option.value === resetValue ? <CommandSeparator /> : null}
                  </React.Fragment>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

interface MultiComboboxProps extends ComboboxSharedProps {
  values: readonly string[];
  onValuesChange: (values: string[]) => void;
  /* Trigger copy once more than one option is on, e.g. "3 estados". Receives the count. */
  summaryLabel: (count: number) => string;
  /* Clears the selection from inside the list. Omitted when there is nothing to clear. */
  clearLabel: string;
}

/*
 * The same dropdown, holding several values at once. It stays open across picks — a filter is
 * adjusted in runs, and a panel that shuts after every tick turns three choices into three trips.
 *
 * An empty selection means "everything", which is why there is no all-sentinel option here: the
 * reset is `clearLabel`, and the trigger falls back to the placeholder on its own.
 */
function MultiCombobox({
  options,
  values,
  onValuesChange,
  placeholder,
  summaryLabel,
  clearLabel,
  emptyLabel,
  searchPlaceholder,
  searchable = false,
  icon,
  disabled = false,
  id,
  className,
  contentClassName,
  'aria-label': ariaLabel,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
}: MultiComboboxProps) {
  const [open, setOpen] = React.useState(false);
  const listRef = React.useRef<HTMLDivElement>(null);
  const trigger = useTriggerWidth();

  const groups = React.useMemo(() => groupOptions(options), [options]);
  const selectedSet = React.useMemo(() => new Set(values), [values]);
  const onlyOne = values.length === 1 ? options.find((o) => o.value === values[0]) : undefined;

  function toggle(value: string) {
    onValuesChange(
      selectedSet.has(value) ? values.filter((item) => item !== value) : [...values, value],
    );
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        if (next) trigger.measure();
        setOpen(next);
      }}
    >
      <PopoverTrigger asChild>
        <ComboboxTrigger
          ref={trigger.ref}
          id={id}
          open={open}
          unset={values.length === 0}
          invalid={ariaInvalid}
          icon={icon}
          label={
            values.length === 0 ? placeholder : (onlyOne?.label ?? summaryLabel(values.length))
          }
          aria-label={ariaLabel}
          aria-describedby={ariaDescribedBy}
          disabled={disabled}
          className={className}
        />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        onOpenAutoFocus={
          searchable
            ? undefined
            : (event) => {
                event.preventDefault();
                listRef.current?.focus();
              }
        }
        style={trigger.style}
        className={cn('w-(--radix-popover-trigger-width) p-0', contentClassName)}
      >
        <Command shouldFilter={searchable}>
          {searchable ? <CommandInput placeholder={searchPlaceholder} /> : null}
          <CommandList ref={listRef}>
            {searchable ? <CommandEmpty>{emptyLabel}</CommandEmpty> : null}
            {groups.map(([group, groupItems]) => (
              <CommandGroup key={group || 'ungrouped'} heading={group || undefined}>
                {groupItems.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={[option.label]}
                    disabled={option.disabled}
                    onSelect={() => toggle(option.value)}
                  >
                    {option.icon}
                    <span className="truncate">{option.label}</span>
                    <SelectedMark selected={selectedSet.has(option.value)} />
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
            {values.length > 0 ? (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem value="__clear__" onSelect={() => onValuesChange([])}>
                    <span className="text-foreground-muted">{clearLabel}</span>
                  </CommandItem>
                </CommandGroup>
              </>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export { Combobox, MultiCombobox };
