'use client';

import { useTranslations } from 'next-intl';
import { useFormContext } from 'react-hook-form';

import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
} from '@repo/ui/components';
import { type SignupValues } from '@/app/(auth)/signup/form-schema';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// Only the name is required: nothing the account does depends on the fiscal fields, so making
// them a gate would stop a registration for no reason.
export function AccountStep() {
  const t = useTranslations('auth.signup');
  const { control } = useFormContext<SignupValues>();

  return (
    <>
      <FormField
        control={control}
        name="accountName"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('accountName.label')}</FormLabel>
            <FormControl>
              <Input
                autoComplete="organization"
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('accountName.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={control}
        name="legalName"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('legalName.label')}</FormLabel>
            <FormControl>
              <Input
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('legalName.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={control}
        name="taxId"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('taxId.label')}</FormLabel>
            <FormControl>
              <Input
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('taxId.placeholder')}
                {...field}
              />
            </FormControl>
            <FormDescription>{t('taxId.hint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
