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
import { PASSWORD_MAX_LENGTH, PASSWORD_MIN_LENGTH } from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

// The step a server-side rejection lands on: the address is the one field registration refuses.
export function AdminStep() {
  const t = useTranslations('auth.signup');
  const tCommon = useTranslations('common');
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

      <FormField
        control={control}
        name="adminPassword"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('adminPassword.label')}</FormLabel>
            <FormControl>
              <Input
                type="password"
                autoComplete="new-password"
                minLength={PASSWORD_MIN_LENGTH}
                maxLength={PASSWORD_MAX_LENGTH}
                placeholder={t('adminPassword.placeholder')}
                passwordToggleLabel={tCommon('form.togglePassword')}
                {...field}
              />
            </FormControl>
            <FormDescription>{t('minLength', { count: PASSWORD_MIN_LENGTH })}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={control}
        name="confirmPassword"
        render={({ field }) => (
          <FormItem>
            <FormLabel required>{t('confirmPassword.label')}</FormLabel>
            <FormControl>
              <Input
                type="password"
                autoComplete="new-password"
                minLength={PASSWORD_MIN_LENGTH}
                maxLength={PASSWORD_MAX_LENGTH}
                placeholder={t('confirmPassword.placeholder')}
                passwordToggleLabel={tCommon('form.togglePassword')}
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
