'use client';

import { useMemo } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
} from '@repo/ui/components';
import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import type { Branch } from '@/lib/api/branches';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface BranchStepProps {
  branch: Branch;
  formId: string;
  onSubmit: (values: BranchValues) => void | Promise<void>;
}

export function BranchStep({ branch, formId, onSubmit }: BranchStepProps) {
  const t = useTranslations('onboarding.branch');
  const tFields = useTranslations('branches');
  const tErrors = useTranslations('common.form.errors');
  const text = useMemo(() => ({ field: tFields, shared: tErrors }), [tFields, tErrors]);
  const form = useForm<BranchValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(branchSchema(text)),
    defaultValues: {
      name: branch.name,
      address: branch.address ?? '',
      defaultExpiryDays: String(branch.defaultExpiryDays),
    },
  });

  return (
    <div className="flex flex-col gap-y-6">
      <Form {...form}>
        <form
          id={formId}
          onSubmit={form.handleSubmit(onSubmit)}
          noValidate
          className="grid gap-5 md:grid-cols-2"
        >
          <FormField
            control={form.control}
            name="name"
            render={({ field }) => (
              <FormItem>
                <FormLabel required>{tFields('name.label')}</FormLabel>
                <FormControl>
                  <Input placeholder={tFields('name.placeholder')} maxLength={255} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="address"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{tFields('address.label')}</FormLabel>
                <FormControl>
                  <Input placeholder={tFields('address.placeholder')} maxLength={255} {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="defaultExpiryDays"
            render={({ field }) => (
              <FormItem className="md:col-span-2">
                <FormLabel required>{tFields('defaultExpiryDays.label')}</FormLabel>
                <FormControl>
                  <Input type="number" inputMode="numeric" className="max-w-44" {...field} />
                </FormControl>
                <FormDescription>{t('expiryHint')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormRootMessage className="md:col-span-2" />
        </form>
      </Form>
    </div>
  );
}
