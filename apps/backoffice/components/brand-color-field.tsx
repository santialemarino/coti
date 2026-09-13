'use client';

import { useTranslations } from 'next-intl';
import type { Control, FieldValues, Path } from 'react-hook-form';

import {
  ColorPicker,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
} from '@repo/ui/components';
import { BRAND_COLOR_PRESETS, DEFAULT_BRAND_COLOR } from '@/lib/constants/brand';

interface BrandColorFieldProps<TValues extends FieldValues> {
  control: Control<TValues>;
  name: Path<TValues>;
  label: string;
  placeholder: string;
  /* Names the picker's trigger; its three inner controls are named from the shared catalog. */
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
  const tPicker = useTranslations('common.colorPicker');

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const value = typeof field.value === 'string' ? field.value : '';

        return (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            <div className="flex items-center gap-x-2.5">
              <FormControl>
                <Input prefix="#" placeholder={placeholder} maxLength={8} {...field} />
              </FormControl>
              <ColorPicker
                value={value || DEFAULT_BRAND_COLOR.slice(1)}
                onValueChange={field.onChange}
                labels={{
                  trigger: pickerLabel,
                  shade: tPicker('shade'),
                  hue: tPicker('hue'),
                  presets: tPicker('presets'),
                }}
                presets={BRAND_COLOR_PRESETS}
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
