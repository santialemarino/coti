'use client';

import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import {
  Controller,
  FormProvider,
  useFormContext,
  useFormState,
  type ControllerProps,
  type FieldPath,
  type FieldValues,
} from 'react-hook-form';

import { cn } from '../lib/utils';
import { Label } from './label';

const Form = FormProvider;

interface FormFieldContextValue<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> {
  name: TName;
}

const FormFieldContext = React.createContext<FormFieldContextValue>({} as FormFieldContextValue);

function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({ ...props }: ControllerProps<TFieldValues, TName>) {
  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller {...props} />
    </FormFieldContext.Provider>
  );
}

interface FormItemContextValue {
  id: string;
  required?: boolean;
}

const FormItemContext = React.createContext<FormItemContextValue>({} as FormItemContextValue);

/*
 * `required` is declared once on the item and read by both the label (which draws the marker) and
 * the control (which gets aria-required), so the two can't disagree about whether a field is
 * required.
 */
function FormItem({
  className,
  required,
  ...props
}: React.ComponentProps<'div'> & { required?: boolean }) {
  const id = React.useId();

  return (
    <FormItemContext.Provider value={{ id, required }}>
      <div data-slot="form-item" className={cn('flex flex-col gap-y-2', className)} {...props} />
    </FormItemContext.Provider>
  );
}

// useFormField wires a field's id, its description and its error message together, so
// the control can point at both without every call site repeating the ids.
function useFormField() {
  const fieldContext = React.useContext(FormFieldContext);
  const itemContext = React.useContext(FormItemContext);
  const { getFieldState } = useFormContext();
  const formState = useFormState({ name: fieldContext.name });
  const fieldState = getFieldState(fieldContext.name, formState);

  if (!fieldContext) {
    throw new Error('useFormField has to be used inside a <FormField>');
  }

  const { id, required } = itemContext;
  return {
    id,
    name: fieldContext.name,
    required,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    ...fieldState,
  };
}

function FormLabel({ className, required, ...props }: React.ComponentProps<typeof Label>) {
  const { error, formItemId, required: fieldRequired } = useFormField();

  return (
    <Label
      data-slot="form-label"
      data-error={!!error}
      required={required ?? fieldRequired}
      className={cn('data-[error=true]:text-danger-foreground', className)}
      htmlFor={formItemId}
      {...props}
    />
  );
}

// FormControl is what makes the invalid styling and the screen-reader wiring automatic:
// it stamps aria-invalid and aria-describedby onto whatever input it wraps.
function FormControl({ ...props }: React.ComponentProps<typeof Slot>) {
  const { error, formItemId, formDescriptionId, formMessageId, required } = useFormField();

  return (
    <Slot
      data-slot="form-control"
      id={formItemId}
      aria-describedby={error ? `${formDescriptionId} ${formMessageId}` : formDescriptionId}
      aria-invalid={!!error}
      aria-required={required || undefined}
      {...props}
    />
  );
}

function FormDescription({ className, ...props }: React.ComponentProps<'p'>) {
  const { formDescriptionId } = useFormField();

  return (
    <p
      data-slot="form-description"
      id={formDescriptionId}
      className={cn('text-paragraph-xs text-foreground-muted', className)}
      {...props}
    />
  );
}

/*
 * The reveal is a `grid-template-rows` transition from 0fr to 1fr, which animates height without
 * needing JS or a measured pixel value.
 *
 * The two spacing classes are load-bearing. The wrapper's `-mt-2` cancels FormItem's `gap-y-2`, and
 * the inner `pt-2` puts that gap back as padding *inside* the animated box. So a collapsed message
 * occupies exactly nothing — without the pair, every field would carry a permanent 8px of empty
 * space below it waiting for an error that usually never comes.
 */
const REVEAL = 'grid grid-rows-[0fr] transition-[grid-template-rows] duration-200 ease-out-soft';

function FormMessage({ className, children, ...props }: React.ComponentProps<'p'>) {
  const { error, formMessageId } = useFormField();
  const body = children ?? error?.message;

  return (
    <div
      data-slot="form-message"
      aria-hidden={!body}
      className={cn(REVEAL, '-mt-2', body && 'grid-rows-[1fr]')}
    >
      <p
        id={formMessageId}
        role="alert"
        className={cn('overflow-hidden pt-2 text-paragraph-xs text-danger-foreground', className)}
        {...props}
      >
        {body}
      </p>
    </div>
  );
}

/*
 * A form-level error, for a rejection that belongs to no single field — a login the API refused
 * without saying which half was wrong. Reads react-hook-form's root error, so setError('root', …) is
 * all a caller needs. The gap it cancels is the form's own, which is wider than a field's.
 */
function FormRootMessage({ className, children, ...props }: React.ComponentProps<'p'>) {
  const {
    formState: { errors },
  } = useFormContext();
  const body = children ?? errors.root?.message;

  return (
    <div
      data-slot="form-root-message"
      aria-hidden={!body}
      className={cn(REVEAL, '-mt-4', body && 'grid-rows-[1fr]')}
    >
      <p
        role="alert"
        className={cn(
          'overflow-hidden pt-4 text-center text-paragraph-sm text-danger-foreground',
          className,
        )}
        {...props}
      >
        {body}
      </p>
    </div>
  );
}

export {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  useFormField,
};
