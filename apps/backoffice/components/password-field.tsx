'use client';

import { useTranslations } from 'next-intl';
import { useWatch, type Control, type FieldPath, type FieldValues } from 'react-hook-form';

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
} from '@repo/ui/components';
import { PasswordMeter } from '@/components/password-meter';
import { PASSWORD_MAX_LENGTH, passwordChecks, SECRET_MAX_LENGTH } from '@/lib/constants/password';

interface PasswordFieldProps<T extends FieldValues> {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  placeholder: string;
  /*
   * A password the caller already has is presented, never chosen: it takes the wider cap the API
   * accepts, carries no meter, and tells the browser to offer the stored one rather than a new one.
   */
  existing?: boolean;
  /* Only under the field where the password is chosen — never under its confirmation. */
  meter?: boolean;
}

/* Every password input in the app, so the cap, the reveal toggle and the meter cannot drift apart. */
export function PasswordField<T extends FieldValues>({
  control,
  name,
  label,
  placeholder,
  existing = false,
  meter = false,
}: PasswordFieldProps<T>) {
  const tCommon = useTranslations('common');
  const value = useWatch({ control, name });

  return (
    <FormField
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FormItem>
          <FormLabel required>{label}</FormLabel>
          <FormControl>
            <Input
              type="password"
              autoComplete={existing ? 'current-password' : 'new-password'}
              maxLength={existing ? SECRET_MAX_LENGTH : PASSWORD_MAX_LENGTH}
              placeholder={placeholder}
              passwordToggleLabel={tCommon('form.togglePassword')}
              {...field}
            />
          </FormControl>
          <FormMessage />
          {meter ? (
            /* Keyed on this field's own rejection, not on the form having been submitted: a wizard
               step advances by submitting, so a submit count would arrive on the last step red. */
            <PasswordMeter
              checks={passwordChecks(typeof value === 'string' ? value : '')}
              invalid={Boolean(fieldState.error)}
            />
          ) : null}
        </FormItem>
      )}
    />
  );
}
