'use client';

import { useMemo } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Badge,
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
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { updateInvoicingSettings } from '@/app/(protected)/settings/invoicing/actions';
import {
  invoicingSettingsSchema,
  type InvoicingSettingsValues,
} from '@/app/(protected)/settings/invoicing/form-schema';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { formatCuit, IVA_CONDITIONS, type InvoicingSettings } from '@/lib/api/invoicing';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { useFormatters } from '@/lib/i18n/formatters';

// Past this many, the toast counts the branches instead of naming them.
const NAMED_BRANCHES_MAX = 2;

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
  const form = useForm<InvoicingSettingsValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: valuesOf(settings),
  });

  async function onSubmit(values: InvoicingSettingsValues) {
    const before = form.formState.defaultValues as InvoicingSettingsValues;
    const result = await updateInvoicingSettings(values);
    if (!result.settings) {
      form.setError('root', { message: message(result.error) });
      return;
    }
    toast.success(savedMessage(before, values));
    form.reset(valuesOf(result.settings));
  }

  function savedMessage(before: InvoicingSettingsValues, after: InvoicingSettingsValues): string {
    const changes: string[] = [];
    if (
      before.address !== after.address ||
      before.grossIncomeRegistration !== after.grossIncomeRegistration ||
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
                className={
                  settings.taxId ? 'text-foreground tabular-nums' : 'text-foreground-subtle'
                }
              >
                {settings.taxId ? formatCuit(settings.taxId) : t('fiscal.missing')}
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
                  <FormLabel>{t('ivaCondition.label')}</FormLabel>
                  <FormControl>
                    <Combobox
                      value={field.value || null}
                      onValueChange={field.onChange}
                      options={IVA_CONDITIONS.filter(
                        (condition) => condition !== 'FINAL_CONSUMER',
                      ).map((condition) => ({
                        value: condition,
                        label: tConditions(condition),
                      }))}
                      placeholder={t('ivaCondition.placeholder')}
                    />
                  </FormControl>
                  <FormDescription>{t('ivaCondition.hint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {(['address', 'grossIncomeRegistration', 'activityStartedOn'] as const).map((name) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t(`profile.${name}`)}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type={name === 'activityStartedOn' ? 'date' : 'text'}
                        maxLength={name === 'address' ? 255 : 50}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
            <FormField
              control={form.control}
              name="pricesIncludeVat"
              render={({ field }) => (
                <FormItem>
                  <div className="flex items-center gap-x-3">
                    <FormControl>
                      <Switch checked={field.value} onCheckedChange={field.onChange} />
                    </FormControl>
                    <FormLabel>{t('pricesIncludeVat.label')}</FormLabel>
                  </div>
                  <FormDescription>{t('pricesIncludeVat.hint')}</FormDescription>
                </FormItem>
              )}
            />
          </section>

          <section className="flex flex-col min-w-0 gap-y-5">
            <div className="flex flex-col gap-y-1">
              <h2 className="text-heading-6">{t('pointsOfSale.title')}</h2>
              <p className="text-paragraph-xs text-foreground-muted">{t('pointsOfSale.hint')}</p>
            </div>
            <Table>
              <TableCaption className="sr-only">{t('pointsOfSale.caption')}</TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead kind="text">{t('pointsOfSale.branch')}</TableHead>
                  <TableHead kind="quantityInput">{t('pointsOfSale.number')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {settings.branches.map((branch) => (
                  <TableRow key={branch.branchId}>
                    <TableCell className="text-paragraph-sm-medium text-foreground">
                      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        {branch.name}
                        {branch.isActive ? null : (
                          <Badge tone="neutral" size="sm">
                            {t('pointsOfSale.closed')}
                          </Badge>
                        )}
                      </span>
                    </TableCell>
                    <TableCell kind="quantity">{branch.pointOfSale ?? '—'}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>
        </div>

        <FormRootMessage />

        <PendingButton
          type="submit"
          className="w-full max-w-md lg:max-w-none"
          pending={form.formState.isSubmitting}
          pendingLabel={t('submitting')}
        >
          {t('submit')}
        </PendingButton>
      </form>
    </Form>
  );
}

export function valuesOf(settings: InvoicingSettings): InvoicingSettingsValues {
  return {
    address: settings.profile?.address ?? '',
    grossIncomeRegistration: settings.profile?.grossIncomeRegistration ?? '',
    activityStartedOn: settings.profile?.activityStartedOn ?? '',
    ivaCondition: settings.ivaCondition ?? '',
    pricesIncludeVat: settings.pricesIncludeVat,
    branches: settings.branches.map((branch) => ({
      branchId: branch.branchId,
      pointOfSale: branch.pointOfSale === null ? '' : String(branch.pointOfSale),
    })),
  };
}
