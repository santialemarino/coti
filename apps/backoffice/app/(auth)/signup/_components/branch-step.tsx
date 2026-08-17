'use client';

import { useTranslations } from 'next-intl';
import { useFormContext } from 'react-hook-form';

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
} from '@repo/ui/components';
import { type SignupValues } from '@/app/(auth)/signup/form-schema';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// The first branch, which registration always creates: everything branch-scoped needs one to
// exist, so there is no account without it.
export function BranchStep() {
  const t = useTranslations('auth.signup');
  const { control } = useFormContext<SignupValues>();

  return (
    <>
      <FormField
        control={control}
        name="branchName"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('branchName.label')}</FormLabel>
            <FormControl>
              <Input
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('branchName.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={control}
        name="branchAddress"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('branchAddress.label')}</FormLabel>
            <FormControl>
              <Input
                autoComplete="street-address"
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('branchAddress.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}
