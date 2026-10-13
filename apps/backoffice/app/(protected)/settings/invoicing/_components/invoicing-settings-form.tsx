'use client';

import { useMemo } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Combobox,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
  Switch,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { GrossIncomeNumberInput } from '@/app/(protected)/settings/invoicing/_components/gross-income-number-input';
import { updateInvoicingSettings } from '@/app/(protected)/settings/invoicing/actions';
import {
  ADDRESS_MAX_LENGTH,
  GROSS_INCOME_EXEMPT,
  GROSS_INCOME_NUMBER_MAX_LENGTH,
  invoicingSettingsSchema,
  ISSUER_IVA_CONDITIONS,
  type GrossIncomeKind,
  type InvoicingSettingsOutput,
  type InvoicingSettingsValues,
} from '@/app/(protected)/settings/invoicing/form-schema';
import { PointsOfSaleTable } from '@/components/points-of-sale-table';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { formatCuit, type InvoicingSettings } from '@/lib/api/invoicing';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { changedPointsOfSale, pointsOfSaleValues } from '@/lib/forms/points-of-sale';
import { useFormatters } from '@/lib/i18n/formatters';

// Past this many, the toast counts the branches instead of naming them.
const NAMED_BRANCHES_MAX = 2;
// The anchor the setup notices and the ARCA guide link to.
const POINTS_OF_SALE_SECTION = 'points-of-sale';
// The worked example under the price switch: an item at 21% IVA.
const VAT_EXAMPLE = { net: '1000', vat: '210', gross: '1210' };

interface InvoicingSettingsFormProps {
  settings: InvoicingSettings;
}

export function InvoicingSettingsForm({ settings }: InvoicingSettingsFormProps) {
  const fmt = useFormatters();
  const t = useTranslations('invoicing.settings');
  const tConditions = useTranslations('invoicing.ivaConditions');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('invoicing.settings');
  const schema = useMemo(
    () => invoicingSettingsSchema({ field: t, shared: tErrors }),
    [t, tErrors],
  );
  const form = useForm<InvoicingSettingsValues, unknown, InvoicingSettingsOutput>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: valuesOf(settings),
  });
  const grossIncomeKind = useWatch({ control: form.control, name: 'grossIncomeKind' });
  const pending = form.formState.isSubmitting;
  const accountCuit = settings.taxId ? formatCuit(settings.taxId.replace(/\D/g, '')) : null;

  async function onSubmit(values: InvoicingSettingsOutput) {
    const before = form.formState.defaultValues as InvoicingSettingsValues;
    const result = await updateInvoicingSettings({
      ...values,
      branches: changedPointsOfSale(values.branches, before.branches),
    });
    if (!result.settings) {
      form.setError('root', { message: message(result.error) });
      return;
    }
    toast.success(savedMessage(before, form.getValues()));
    form.reset(valuesOf(result.settings));
  }

  function savedMessage(before: InvoicingSettingsValues, after: InvoicingSettingsValues): string {
    const changes: string[] = [];
    if (
      before.address !== after.address ||
      before.grossIncomeKind !== after.grossIncomeKind ||
      before.grossIncomeNumber !== after.grossIncomeNumber ||
      before.activityStartedOn !== after.activityStartedOn
    )
      changes.push(t('changes.profile'));
    if (before.ivaCondition !== after.ivaCondition) changes.push(t('changes.ivaCondition'));
    if (before.pricesIncludeVat !== after.pricesIncludeVat) {
      changes.push(t('changes.pricesIncludeVat'));
    }
    const moved = settings.branches
      .filter(
        (_, index) => before.branches[index]?.pointOfSale !== after.branches[index]?.pointOfSale,
      )
      .map((branch) => branch.name);
    if (moved.length > NAMED_BRANCHES_MAX) {
      changes.push(t('changes.pointsOfSale', { count: moved.length }));
    } else if (moved.length > 0) {
      changes.push(t('changes.pointOfSale', { branches: fmt.list(moved) }));
    }
    return changes.length > 0 ? t('saved', { changes: fmt.list(changes) }) : t('savedUnchanged');
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        noValidate
        className="flex flex-col w-full gap-y-6 lg:gap-y-8"
      >
        <div className="grid grid-cols-1 gap-x-12 gap-y-8 lg:grid-cols-2">
          <section className="flex flex-col max-w-md gap-y-5 lg:max-w-none">
            <h2 className="text-heading-6">{t('fiscal.title')}</h2>

            <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-paragraph-sm">
              <dt className="text-foreground-muted">{t('fiscal.legalName')}</dt>
              <dd className={settings.legalName ? 'text-foreground' : 'text-foreground-subtle'}>
                {settings.legalName ?? t('fiscal.missing')}
              </dd>
              <dt className="text-foreground-muted">{t('fiscal.taxId')}</dt>
              <dd
                className={accountCuit ? 'text-foreground tabular-nums' : 'text-foreground-subtle'}
              >
                {accountCuit ?? t('fiscal.missing')}
              </dd>
            </dl>
            <p className="text-paragraph-xs text-foreground-muted">
              {t('fiscal.editHint')}{' '}
              <InlineLink asChild>
                <Link href={ROUTES.accountSettings}>{t('fiscal.editLink')}</Link>
              </InlineLink>
            </p>

            <FormField
              control={form.control}
              name="ivaCondition"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('ivaCondition.label')}</FormLabel>
                  <FormControl>
                    <Combobox
                      ref={field.ref}
                      value={field.value || null}
                      onValueChange={field.onChange}
                      options={ISSUER_IVA_CONDITIONS.map((condition) => ({
                        value: condition,
                        label: tConditions(condition),
                      }))}
                      placeholder={t('ivaCondition.placeholder')}
                      disabled={pending}
                    />
                  </FormControl>
                  <FormDescription>{t('ivaCondition.hint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="address"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('profile.address')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      maxLength={ADDRESS_MAX_LENGTH}
                      placeholder={t('profile.addressPlaceholder')}
                      disabled={pending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="grossIncomeKind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('profile.grossIncome')}</FormLabel>
                  <FormControl>
                    <ToggleGroup
                      type="single"
                      value={field.value}
                      onValueChange={(value) => value && field.onChange(value as GrossIncomeKind)}
                      disabled={pending}
                      aria-label={t('profile.grossIncome')}
                    >
                      <ToggleGroupItem value="number">
                        {t('profile.grossIncomeKinds.number')}
                      </ToggleGroupItem>
                      <ToggleGroupItem value="exempt">
                        {t('profile.grossIncomeKinds.exempt')}
                      </ToggleGroupItem>
                    </ToggleGroup>
                  </FormControl>
                  {grossIncomeKind === 'exempt' ? (
                    <FormDescription>{t('profile.grossIncomeExemptHint')}</FormDescription>
                  ) : null}
                </FormItem>
              )}
            />

            {grossIncomeKind === 'number' ? (
              <FormField
                control={form.control}
                name="grossIncomeNumber"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('profile.grossIncomeNumber')}</FormLabel>
                    <FormControl>
                      <GrossIncomeNumberInput
                        {...field}
                        maxLength={GROSS_INCOME_NUMBER_MAX_LENGTH}
                        placeholder={t('profile.grossIncomeNumberPlaceholder')}
                        disabled={pending}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('profile.grossIncomeNumberHint')}
                      {accountCuit ? (
                        <>
                          {' '}
                          {t('profile.sameAsCuit')}{' '}
                          <InlineLink asChild>
                            <button
                              type="button"
                              disabled={pending}
                              onClick={() =>
                                form.setValue('grossIncomeNumber', accountCuit, {
                                  shouldDirty: true,
                                  shouldValidate: form.formState.isSubmitted,
                                })
                              }
                            >
                              {t('profile.useCuit')}
                            </button>
                          </InlineLink>
                        </>
                      ) : null}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}

            <FormField
              control={form.control}
              name="activityStartedOn"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('profile.activityStartedOn')}</FormLabel>
                  <FormControl>
                    <Input {...field} type="date" disabled={pending} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="pricesIncludeVat"
              render={({ field }) => (
                <FormItem>
                  <div className="flex items-center gap-x-3">
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        disabled={pending}
                      />
                    </FormControl>
                    <FormLabel>{t('pricesIncludeVat.label')}</FormLabel>
                  </div>
                  <FormDescription>
                    {t(field.value ? 'pricesIncludeVat.hintOn' : 'pricesIncludeVat.hintOff', {
                      net: fmt.currency(VAT_EXAMPLE.net),
                      vat: fmt.currency(VAT_EXAMPLE.vat),
                      gross: fmt.currency(VAT_EXAMPLE.gross),
                    })}
                  </FormDescription>
                </FormItem>
              )}
            />
          </section>

          <section
            id={POINTS_OF_SALE_SECTION}
            aria-labelledby={`${POINTS_OF_SALE_SECTION}-title`}
            className="flex flex-col min-w-0 gap-y-5 scroll-mt-24"
          >
            <div className="flex flex-col gap-y-1">
              <h2 id={`${POINTS_OF_SALE_SECTION}-title`} className="text-heading-6">
                {t('pointsOfSale.title')}
              </h2>
              <p className="text-paragraph-xs text-foreground-muted">{t('pointsOfSale.hint')}</p>
            </div>
            <PointsOfSaleTable
              control={form.control}
              branches={settings.branches}
              disabled={pending}
            />
          </section>
        </div>

        <FormRootMessage />

        <PendingButton
          type="submit"
          className="w-full sm:w-auto sm:self-start"
          pending={pending}
          pendingLabel={t('submitting')}
        >
          {t('submit')}
        </PendingButton>
      </form>
    </Form>
  );
}

export function valuesOf(settings: InvoicingSettings): InvoicingSettingsValues {
  const registration = settings.profile?.grossIncomeRegistration ?? '';
  const exempt = registration.toUpperCase() === GROSS_INCOME_EXEMPT;
  return {
    address: settings.profile?.address ?? '',
    grossIncomeKind: exempt ? 'exempt' : 'number',
    grossIncomeNumber: exempt ? '' : registration,
    activityStartedOn: settings.profile?.activityStartedOn ?? '',
    ivaCondition:
      settings.ivaCondition && settings.ivaCondition !== 'FINAL_CONSUMER'
        ? settings.ivaCondition
        : '',
    pricesIncludeVat: settings.pricesIncludeVat,
    branches: pointsOfSaleValues(settings.branches),
  };
}
