'use client';

import { useMemo, useState } from 'react';
import Image from 'next/image';
import { zodResolver } from '@hookform/resolvers/zod';
import { Building2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';

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
import { LogoDropzone } from '@/app/(onboarding)/onboarding/_components/logo-dropzone';
import {
  onboardingBrandSchema,
  type OnboardingBrandValues,
} from '@/app/(onboarding)/onboarding/form-schema';
import type { Account } from '@/lib/api/account';
import { DEFAULT_BRAND_COLOR, HEX_COLOR_DIGITS } from '@/lib/constants/brand';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface BrandStepProps {
  account: Account;
  formId: string;
  onSubmit: (values: OnboardingBrandValues) => void | Promise<void>;
}

export function BrandStep({ account, formId, onSubmit }: BrandStepProps) {
  const t = useTranslations('onboarding.brand');
  const tErrors = useTranslations('common.form.errors');
  const text = useMemo(() => ({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<OnboardingBrandValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(onboardingBrandSchema(text)),
    defaultValues: { brandColor: account.brandColor?.replace(/^#/, '') ?? '' },
  });
  const [logoPreviewUrl, setLogoPreviewUrl] = useState<string | null>(null);
  const brandColor = useWatch({ control: form.control, name: 'brandColor' });
  const previewColor = HEX_COLOR_DIGITS.test(brandColor) ? `#${brandColor}` : null;
  const pickerColor = /^[0-9a-f]{6}$/i.test(brandColor) ? `#${brandColor}` : DEFAULT_BRAND_COLOR;

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_20rem]">
      <div className="flex flex-col gap-y-6">
        <div className="flex flex-col p-5 gap-y-2 bg-muted border rounded-lg">
          <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">
            {t('accountSummary')}
          </p>
          <p className="text-heading-5">{account.name}</p>
          <p className="text-paragraph-sm text-foreground-muted">
            {[account.legalName, account.taxId].filter(Boolean).join(' · ') || t('noLegalData')}
          </p>
        </div>

        <LogoDropzone onPreviewChange={setLogoPreviewUrl} />

        <Form {...form}>
          <form id={formId} onSubmit={form.handleSubmit(onSubmit)} noValidate>
            <FormField
              control={form.control}
              name="brandColor"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('brandColor.label')}</FormLabel>
                  <div className="flex items-center gap-x-2.5">
                    <FormControl>
                      <Input
                        prefix="#"
                        placeholder={t('brandColor.placeholder')}
                        maxLength={8}
                        {...field}
                      />
                    </FormControl>
                    <input
                      type="color"
                      aria-label={t('brandColor.pickerLabel')}
                      value={pickerColor}
                      className="size-9 shrink-0 p-1 bg-input border border-border rounded-lg outline-none shadow-e1 transition-[border-color,box-shadow,scale] duration-200 ease-out-soft hover:border-strong active:scale-[0.98] focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45"
                      onInput={(event) =>
                        field.onChange(event.currentTarget.value.slice(1).toUpperCase())
                      }
                    />
                  </div>
                  <FormDescription>{t('brandColor.hint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormRootMessage />
          </form>
        </Form>
      </div>

      <aside className="flex flex-col p-5 gap-y-5 bg-card border rounded-1.5xl shadow-e2">
        <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">{t('preview')}</p>
        <div className="overflow-hidden border rounded-lg">
          <div
            className="h-2 bg-primary"
            style={previewColor ? { backgroundColor: previewColor } : undefined}
          />
          <div className="flex flex-col p-5 gap-y-6">
            {logoPreviewUrl ? (
              <div className="relative h-14 w-40">
                <Image
                  src={logoPreviewUrl}
                  alt={t('logo.previewAlt')}
                  fill
                  unoptimized
                  className="object-contain object-left"
                />
              </div>
            ) : (
              <span className="flex size-10 items-center justify-center bg-accent rounded-lg text-accent-foreground">
                <Building2Icon aria-hidden="true" className="size-5" />
              </span>
            )}
            <div className="flex flex-col gap-y-1">
              <p className="text-heading-6">{account.name}</p>
              <p className="text-paragraph-xs text-foreground-muted">{t('quotePreview')}</p>
            </div>
            <div className="h-16 bg-muted rounded-lg" />
          </div>
        </div>
      </aside>
    </div>
  );
}
