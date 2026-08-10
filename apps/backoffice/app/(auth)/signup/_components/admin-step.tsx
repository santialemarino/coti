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
import { PasswordField } from '@/components/password-field';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// The step a server-side rejection lands on: the address is the one field registration refuses.
export function AdminStep() {
  const t = useTranslations('auth.signup');
  const { control } = useFormContext<SignupValues>();

  return (
    <>
      <FormField
        control={control}
        name="adminName"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('adminName.label')}</FormLabel>
            <FormControl>
              <Input
                autoComplete="name"
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('adminName.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={control}
        name="adminEmail"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('adminEmail.label')}</FormLabel>
            <FormControl>
              <Input
                type="email"
                autoComplete="email"
                maxLength={TEXT_FIELD_MAX_LENGTH}
                placeholder={t('adminEmail.placeholder')}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      <PasswordField
        control={control}
        name="adminPassword"
        label={t('adminPassword.label')}
        placeholder={t('adminPassword.placeholder')}
        meter
      />

      <PasswordField
        control={control}
        name="confirmPassword"
        label={t('confirmPassword.label')}
        placeholder={t('confirmPassword.placeholder')}
      />
    </>
  );
}
