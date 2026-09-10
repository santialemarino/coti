'use client';

import { useMemo, useState } from 'react';
import Image from 'next/image';
import { zodResolver } from '@hookform/resolvers/zod';
import { Building2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';

import { Card, Form, FormRootMessage } from '@repo/ui/components';
import {
  onboardingBrandSchema,
  type OnboardingBrandValues,
} from '@/app/(onboarding)/onboarding/form-schema';
import { BrandColorField } from '@/components/brand-color-field';
import { LogoDropzone } from '@/components/logo-dropzone';
import type { Account } from '@/lib/api/account';
import { HEX_COLOR_DIGITS } from '@/lib/constants/brand';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface BrandStepProps {
  account: Account;
  formId: string;
  onSubmit: (values: OnboardingBrandValues, logo?: File | null) => void | Promise<void>;
}

export function BrandStep({ account, formId, onSubmit }: BrandStepProps) {
  const t = useTranslations('onboarding.brand');
  const tLogo = useTranslations('common.logoUpload');
  const tErrors = useTranslations('common.form.errors');
  const text = useMemo(() => ({ field: t, shared: tErrors }), [t, tErrors]);
  const form = useForm<OnboardingBrandValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(onboardingBrandSchema(text)),
    defaultValues: { brandColor: account.brandColor?.replace(/^#/, '') ?? '' },
  });
  const [logo, setLogo] = useState<File | null>();
  const [logoPreviewUrl, setLogoPreviewUrl] = useState<string | null>(null);
  const brandColor = useWatch({ control: form.control, name: 'brandColor' });
  const previewColor = HEX_COLOR_DIGITS.test(brandColor) ? `#${brandColor}` : null;

  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_20rem]">
      <div className="flex flex-col gap-y-6">
        <Card className="p-5 gap-y-2 bg-muted rounded-lg shadow-e1">
          <p className="text-paragraph-xs-medium text-foreground-subtle uppercase">
            {t('accountSummary')}
          </p>
          <p className="text-heading-5">{account.name}</p>
          <p className="text-paragraph-sm text-foreground-muted">
            {[account.legalName, account.taxId].filter(Boolean).join(' · ') || t('noLegalData')}
          </p>
        </Card>

        <LogoDropzone
          initialUrl={account.brandLogoUrl}
          onFileChange={setLogo}
          onPreviewChange={setLogoPreviewUrl}
        />

        <Form {...form}>
          <form
            id={formId}
            onSubmit={form.handleSubmit((values) => onSubmit(values, logo))}
            noValidate
          >
            <BrandColorField
              control={form.control}
              name="brandColor"
              label={t('brandColor.label')}
              placeholder={t('brandColor.placeholder')}
              pickerLabel={t('brandColor.pickerLabel')}
              hint={t('brandColor.hint')}
            />
            <FormRootMessage />
          </form>
        </Form>
      </div>

      <Card className="self-start p-5 gap-y-5">
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
                  alt={tLogo('previewAlt')}
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
          </div>
        </div>
      </Card>
    </div>
  );
}
