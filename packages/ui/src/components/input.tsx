'use client';

import * as React from 'react';
import { Eye, EyeOff } from 'lucide-react';

import { cn } from '../lib/utils';

interface InputProps extends Omit<React.ComponentProps<'input'>, 'prefix'> {
  /* Rendered inside the field, before the text. Decorative — give it no interactivity. */
  startIcon?: React.ReactNode;
  /* Rendered inside the field, after the text. Ignored for `type="password"`, which owns that slot. */
  endIcon?: React.ReactNode;
  /* Static text against the field's left edge (a currency symbol, a unit). */
  prefix?: React.ReactNode;
  /* Static text against the field's right edge. */
  suffix?: React.ReactNode;
  containerClassName?: string;
  /* Accessible name for the password reveal toggle. Pass a translated string. */
  passwordToggleLabel?: string;
}

/*
 * The focus ring lives on the container via `focus-within`, not on the `<input>`, because the field
 * is a composite: an icon, the input, and possibly a reveal toggle all sit inside one bordered box,
 * and the ring has to trace that box rather than the text node. The inner input therefore suppresses
 * its own ring explicitly.
 *
 * `type="password"` grows a reveal toggle. The two icons are stacked in a single grid cell and
 * crossfaded rather than swapped, so the control never reflows mid-toggle.
 */
const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      containerClassName,
      startIcon,
      endIcon,
      prefix,
      suffix,
      type,
      passwordToggleLabel,
      ...props
    },
    ref,
  ) => {
    const [revealed, setRevealed] = React.useState(false);

    const isPassword = type === 'password';
    const hasError = props['aria-invalid'] === true || props['aria-invalid'] === 'true';
    const resolvedType = isPassword && revealed ? 'text' : type;

    return (
      <div
        data-slot="input-container"
        className={cn(
          'relative flex h-9 w-full items-center border border-border rounded-lg shadow-e1',
          'transition-[border-color,box-shadow] duration-200 ease-out-soft',
          props.readOnly ? 'bg-input-readonly' : 'bg-input',
          'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/45',
          hasError && 'border-danger focus-within:border-danger focus-within:ring-danger/30',
          props.disabled && 'opacity-50',
          containerClassName,
        )}
      >
        {prefix ? (
          <span className="shrink-0 pl-3 pointer-events-none text-paragraph-sm text-foreground-muted">
            {prefix}
          </span>
        ) : null}

        {startIcon && !prefix ? (
          <span className="absolute left-3 flex items-center pointer-events-none text-foreground-subtle">
            {startIcon}
          </span>
        ) : null}

        <input
          ref={ref}
          type={resolvedType}
          data-slot="input"
          className={cn(
            'h-full w-full min-w-0 px-3 py-1 bg-transparent border-0 rounded-lg outline-none',
            'text-paragraph-sm text-foreground placeholder:text-foreground-subtle',
            'disabled:pointer-events-none disabled:cursor-not-allowed',
            'focus-visible:outline-none focus-visible:ring-0',
            'file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-paragraph-sm-medium file:text-foreground',
            /* Chrome paints its own yellow fill on autofill; an enormous inset shadow is the only
               way to cover it, and the absurd transition delay stops it flashing back. */
            '[&:-webkit-autofill]:[-webkit-box-shadow:0_0_0_1000px_var(--input)_inset]',
            '[&:-webkit-autofill]:[transition:background-color_9999s_ease-in-out_0s]',
            startIcon && !prefix && 'pl-10',
            prefix && 'pl-2',
            (isPassword || endIcon) && 'pr-10',
            suffix && 'pr-2',
            className,
          )}
          {...props}
        />

        {endIcon && !isPassword ? (
          <span className="absolute right-3 flex items-center text-foreground-subtle">
            {endIcon}
          </span>
        ) : null}

        {isPassword ? (
          <button
            type="button"
            onClick={() => setRevealed((prev) => !prev)}
            aria-label={passwordToggleLabel}
            aria-pressed={revealed}
            tabIndex={props.disabled ? -1 : 0}
            className={cn(
              /* Icon-only trigger: no rectangular ring. Focus is a colour shift plus the bump, so
                 the affordance still reads when reduced motion removes the bump. */
              'group/reveal absolute right-2 flex items-center justify-center p-1 rounded-md outline-none',
              'transition-colors duration-200 ease-out-soft',
              hasError
                ? 'text-danger'
                : 'text-foreground-subtle hover:text-foreground focus-visible:text-foreground',
            )}
          >
            <span className="grid group-focus-visible/reveal:animate-focus-bump">
              <Eye
                aria-hidden="true"
                className={cn(
                  'col-start-1 row-start-1 size-4 transition-[opacity,transform] duration-200 ease-out-soft',
                  revealed ? 'scale-0 opacity-0' : 'scale-100 opacity-100',
                )}
              />
              <EyeOff
                aria-hidden="true"
                className={cn(
                  'col-start-1 row-start-1 size-4 transition-[opacity,transform] duration-200 ease-out-soft',
                  revealed ? 'scale-100 opacity-100' : 'scale-0 opacity-0',
                )}
              />
            </span>
          </button>
        ) : null}

        {suffix ? (
          <span className="shrink-0 pr-3 pointer-events-none text-paragraph-sm text-foreground-muted">
            {suffix}
          </span>
        ) : null}
      </div>
    );
  },
);

Input.displayName = 'Input';

export { Input };
