'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { ReceiptTextIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Combobox,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import {
  clientFiscalSchema,
  type ClientFiscalValues,
} from '@/app/(protected)/clients/[id]/form-schema';
import { TaxIdInput } from '@/components/tax-id-input';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { updateClientFiscal } from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';
import { formatCuit, IVA_CONDITIONS, type ClientFiscal } from '@/lib/api/invoicing';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { CUIT_DIGITS } from '@/lib/utils/cuit';
import { taxIdDigits } from '@/lib/utils/tax-id';

interface ClientFiscalFormProps {
  client: ClientFiscal;
}

/*
 * Fiscal identity belongs to the client profile and is shared by its orders. The CUIT is read-only:
 * only the customer loads it, by creating their account. A DNI stays editable for a consumidor
 * final without one.
 */
export function ClientFiscalForm({ client }: ClientFiscalFormProps) {
  const t = useTranslations('clients.fiscal');
  const tConditions = useTranslations('invoicing.ivaConditions');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('clients.fiscal');
  const schema = useMemo(() => clientFiscalSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const router = useRouter();
  const form = useForm<ClientFiscalValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: valuesOf(client),
  });
  const cuit = cuitOf(client.taxId);

  async function onSubmit(values: ClientFiscalValues) {
    try {
      // Omitted rather than empty: an omitted field is how the API clears it.
      const saved = await updateClientFiscal(client.id, {
        address: values.address || undefined,
        legalName: values.legalName || undefined,
        dni: cuit ? undefined : values.dni || undefined,
        ivaCondition: values.ivaCondition || undefined,
      });
      const name = saved.legalName ?? saved.name;
      toast.success(name ? t('saved', { name }) : t('savedUnnamed'));
      form.reset(valuesOf(saved));
      router.refresh();
    } catch (cause) {
      const code = errorCodeOf(cause);
      if (code === 'INVALID_TAX_ID') {
        form.setError('dni', { message: message(code) });
        return;
      }
      form.setError('root', { message: message(code) });
    }
  }

  return (
    <Card id="fiscal-data">
      <CardHeader>
        <div className="flex items-center gap-x-2">
          <ReceiptTextIcon aria-hidden="true" className="size-4 text-foreground-muted" />
          <CardTitle>{t('title')}</CardTitle>
        </div>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            noValidate
            aria-label={t('title')}
            className="flex flex-col gap-y-5"
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="address"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('address.label')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        maxLength={TEXT_FIELD_MAX_LENGTH}
                        placeholder={t('address.placeholder')}
                        disabled={form.formState.isSubmitting}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="legalName"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('legalName.label')}</FormLabel>
                    <FormControl>
                      <Input
                        disabled={form.formState.isSubmitting}
                        maxLength={TEXT_FIELD_MAX_LENGTH}
                        placeholder={t('legalName.placeholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <dl className="flex flex-col gap-y-2">
                <dt className="text-paragraph-sm-medium text-foreground">{t('cuit.label')}</dt>
                <dd className="text-paragraph-sm text-foreground">
                  {cuit ? formatCuit(cuit) : t('cuit.missing')}
                </dd>
                <dd className="text-paragraph-xs text-foreground-muted">{t('cuit.hint')}</dd>
              </dl>
              {cuit ? null : (
                <FormField
                  control={form.control}
                  name="dni"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('dni.label')}</FormLabel>
                      <FormControl>
                        <TaxIdInput
                          kind="dni"
                          disabled={form.formState.isSubmitting}
                          placeholder={t('dni.placeholder')}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>{t('dni.hint')}</FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              <FormField
                control={form.control}
                name="ivaCondition"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('ivaCondition.label')}</FormLabel>
                    <FormControl>
                      <Combobox
                        ref={field.ref}
                        disabled={form.formState.isSubmitting}
                        value={field.value || null}
                        onValueChange={field.onChange}
                        options={IVA_CONDITIONS.map((condition) => ({
                          value: condition,
                          label: tConditions(condition),
                        }))}
                        placeholder={t('ivaCondition.placeholder')}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormRootMessage />
            <div className="flex justify-end">
              <PendingButton
                type="submit"
                pending={form.formState.isSubmitting}
                pendingLabel={t('submitting')}
              >
                {t('submit')}
              </PendingButton>
            </div>
          </form>
        </Form>
      </CardContent>
    </Card>
  );
}

function valuesOf(client: ClientFiscal): ClientFiscalValues {
  const digits = taxIdDigits(client.taxId ?? '');
  return {
    address: client.address ?? '',
    legalName: client.legalName ?? '',
    dni: cuitOf(digits) ? '' : digits,
    ivaCondition: client.ivaCondition ?? '',
  };
}

// The client's CUIT or CUIL, when the tax id on file is one rather than a DNI.
function cuitOf(taxId: string | null): string | null {
  const digits = taxIdDigits(taxId ?? '');
  return digits.length === CUIT_DIGITS ? digits : null;
}
