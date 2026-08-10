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

  /* Both contexts default to an empty object, so the useful test is whether they were populated —
     and it has to come before the reads below, which would otherwise subscribe to a nameless field
     and mint `undefined-form-item` ids. */
  if (!fieldContext.name || !itemContext.id) {
    throw new Error('useFormField has to be used inside a <FormField> and a <FormItem>');
  }

  const { getFieldState } = useFormContext();
  const formState = useFormState({ name: fieldContext.name });
  const fieldState = getFieldState(fieldContext.name, formState);

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
 * The reveal is a `grid-template-rows` transition from 0fr to 1fr, which animates height without JS
 * and without a measured pixel value.
 *
 * Three things are load-bearing, and the third is easy to get wrong. The wrapper's negative margin
 * cancels the parent column's gap; the padding puts that gap back *inside* the animated box; and the
 * padding must sit on a **descendant** of the grid item, not on the item itself. `overflow: hidden`
 * only suppresses a grid item's automatic minimum size for its *content* — its own padding still
 * floors the row, so padding on the item leaves the collapsed message occupying exactly that padding
 * instead of nothing, and every field keeps a permanent band of empty space waiting for an error that
 * usually never comes.
 */
const REVEAL =
  'grid grid-rows-[0fr] transition-[grid-template-rows] duration-200 ease-out-soft [&>*]:overflow-hidden';

interface RevealedMessageProps extends React.ComponentProps<'p'> {
  slot: string;
  body: React.ReactNode;
  /* The gap this cancels: a field's own, or the wider one between a form's rows. */
  spacing: 'field' | 'form';
}

/*
 * A height animation is only half of an exit. `body` empties in the same commit the row starts
 * collapsing, so the words would vanish on frame one while an empty box shrank — which is what "the
 * error disappearing is not animated" looks like from the outside, even with the height animation
 * working perfectly. The last body is held and faded out with the box instead, and `aria-hidden`
 * keeps the held copy off the accessibility tree while it leaves.
 */
function RevealedMessage({ slot, body, spacing, className, ...props }: RevealedMessageProps) {
  const lastBody = React.useRef(body);
  if (body) lastBody.current = body;

  return (
    <div
      data-slot={slot}
      aria-hidden={!body}
      className={cn(REVEAL, spacing === 'form' ? '-mt-4' : '-mt-2', body && 'grid-rows-[1fr]')}
    >
      <div>
        <p
          role="alert"
          className={cn(
            'transition-opacity duration-200 ease-out-soft',
            spacing === 'form' ? 'pt-4 text-center text-paragraph-sm' : 'pt-2 text-paragraph-xs',
            'text-danger-foreground',
            !body && 'opacity-0',
            className,
          )}
          {...props}
        >
          {body ?? lastBody.current}
        </p>
      </div>
    </div>
  );
}

function FormMessage({ children, ...props }: React.ComponentProps<'p'>) {
  const { error, formMessageId } = useFormField();

  return (
    <RevealedMessage
      slot="form-message"
      body={children ?? error?.message}
      spacing="field"
      id={formMessageId}
      {...props}
    />
  );
}

/*
 * A form-level error, for a rejection that belongs to no single field — a login the API refused
 * without saying which half was wrong. Reads react-hook-form's root error, so setError('root', …) is
 * all a caller needs.
 */
function FormRootMessage({ children, ...props }: React.ComponentProps<'p'>) {
  const {
    formState: { errors },
  } = useFormContext();

  return (
    <RevealedMessage
      slot="form-root-message"
      body={children ?? errors.root?.message}
      spacing="form"
      {...props}
    />
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
