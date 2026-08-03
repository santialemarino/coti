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

const FormItemContext = React.createContext<{ id: string }>({} as { id: string });

function FormItem({ className, ...props }: React.ComponentProps<'div'>) {
  const id = React.useId();

  return (
    <FormItemContext.Provider value={{ id }}>
      <div data-slot="form-item" className={cn('grid gap-y-2', className)} {...props} />
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

  const { id } = itemContext;
  return {
    id,
    name: fieldContext.name,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    ...fieldState,
  };
}

function FormLabel({ className, ...props }: React.ComponentProps<typeof Label>) {
  const { error, formItemId } = useFormField();

  return (
    <Label
      data-slot="form-label"
      data-error={!!error}
      className={cn('data-[error=true]:text-destructive', className)}
      htmlFor={formItemId}
      {...props}
    />
  );
}

// FormControl is what makes the invalid styling and the screen-reader wiring automatic:
// it stamps aria-invalid and aria-describedby onto whatever input it wraps.
function FormControl({ ...props }: React.ComponentProps<typeof Slot>) {
  const { error, formItemId, formDescriptionId, formMessageId } = useFormField();

  return (
    <Slot
      data-slot="form-control"
      id={formItemId}
      aria-describedby={error ? `${formDescriptionId} ${formMessageId}` : formDescriptionId}
      aria-invalid={!!error}
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
      className={cn('text-sm text-muted-foreground', className)}
      {...props}
    />
  );
}

/*
 * The field's error. The grid row animates between 0fr and 1fr, so the message
 * reveals and hides without the layout snapping and reserves no space when there is
 * nothing to say. Pass children to override the field error, which is how a
 * translated message reaches it.
 */
function FormMessage({ className, children, ...props }: React.ComponentProps<'p'>) {
  const { error, formMessageId } = useFormField();
  const body = children ?? error?.message;

  return (
    <div
      data-slot="form-message"
      aria-hidden={!body}
      className={cn(
        'grid grid-rows-[0fr] transition-[grid-template-rows] duration-200 ease-out',
        body && 'grid-rows-[1fr]',
      )}
    >
      <p
        id={formMessageId}
        role="alert"
        className={cn('overflow-hidden text-sm text-destructive', className)}
        {...props}
      >
        {body}
      </p>
    </div>
  );
}

/*
 * A form-level error, for a rejection that belongs to no single field — a login the
 * API refused without saying which half was wrong. Reads react-hook-form's root
 * error, so setError('root', …) is all a caller needs.
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
      className={cn(
        'grid grid-rows-[0fr] transition-[grid-template-rows] duration-200 ease-out',
        body && 'grid-rows-[1fr]',
      )}
    >
      <p
        role="alert"
        className={cn('overflow-hidden text-sm text-destructive', className)}
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
