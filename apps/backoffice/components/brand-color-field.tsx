'use client';

import type { Control, FieldValues, Path } from 'react-hook-form';

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
} from '@repo/ui/components';
import { DEFAULT_BRAND_COLOR } from '@/lib/constants/brand';

interface BrandColorFieldProps<TValues extends FieldValues> {
  control: Control<TValues>;
  name: Path<TValues>;
  label: string;
  placeholder: string;
  pickerLabel: string;
  hint?: string;
}

export function BrandColorField<TValues extends FieldValues>({
  control,
  name,
  label,
  placeholder,
  pickerLabel,
  hint,
}: BrandColorFieldProps<TValues>) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const value = typeof field.value === 'string' ? field.value : '';
        const pickerColor = /^[0-9a-f]{6}$/i.test(value) ? `#${value}` : DEFAULT_BRAND_COLOR;

        return (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            <div className="flex items-center gap-x-2.5">
              <FormControl>
                <Input prefix="#" placeholder={placeholder} maxLength={8} {...field} />
              </FormControl>
              <input
                type="color"
                aria-label={pickerLabel}
                value={pickerColor}
                className="size-9 shrink-0 p-1 bg-input border border-border rounded-lg outline-none shadow-e1 transition-[border-color,box-shadow,scale] duration-200 ease-out-soft hover:border-strong active:scale-[0.98] focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 [&::-moz-color-swatch]:border-0 [&::-moz-color-swatch]:rounded-md [&::-webkit-color-swatch]:border-0 [&::-webkit-color-swatch]:rounded-md [&::-webkit-color-swatch-wrapper]:p-0"
                onInput={(event) =>
                  field.onChange(event.currentTarget.value.slice(1).toUpperCase())
                }
              />
            </div>
            {hint ? <FormDescription>{hint}</FormDescription> : null}
            <FormMessage />
          </FormItem>
        );
      }}
    />
  );
}
