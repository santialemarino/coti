'use client';

import { useEffect, useMemo, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Callout,
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
  Spinner,
} from '@repo/ui/components';
import {
  clientFiscalSchema,
  TAX_ID_MAX_LENGTH,
  type ClientFiscalValues,
} from '@/app/(protected)/rfqs/[id]/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { errorCodeOf } from '@/lib/api/errors';
import { IVA_CONDITIONS, type ClientFiscal } from '@/lib/api/invoicing';
import { getClientFiscal, updateClientFiscal } from '@/lib/api/invoicing-client';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ClientFiscalFormProps {
  clientId: string;
  onSaved: () => void;
}

// The client's fiscal data, edited in place when the invoice cannot go out without it.
export function ClientFiscalForm({ clientId, onSaved }: ClientFiscalFormProps) {
  const t = useTranslations('invoicing.card.fiscal');
  const tConditions = useTranslations('invoicing.ivaConditions');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('invoicing.card');
  const schema = useMemo(() => clientFiscalSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [client, setClient] = useState<ClientFiscal | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const form = useForm<ClientFiscalValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { legalName: '', taxId: '', ivaCondition: '' },
  });

  useEffect(() => {
    let active = true;
    getClientFiscal(clientId)
      .then((fiscal) => {
        if (!active) return;
        setClient(fiscal);
        form.reset({
          legalName: fiscal.legalName ?? '',
          taxId: fiscal.taxId ?? '',
          ivaCondition: fiscal.ivaCondition ?? '',
        });
      })
      .catch((cause) => {
        if (active) setLoadError(message(errorCodeOf(cause)));
      });
    return () => {
      active = false;
    };
  }, [clientId, form, message]);

  async function onSubmit(values: ClientFiscalValues) {
    try {
      // Omitted rather than empty: an omitted field is how the API clears it.
      const saved = await updateClientFiscal(clientId, {
        legal_name: values.legalName || undefined,
        tax_id: values.taxId || undefined,
        iva_condition: values.ivaCondition || undefined,
      });
      const name = saved.legalName ?? saved.name;
      toast.success(name ? t('saved', { name }) : t('savedUnnamed'));
      onSaved();
    } catch (cause) {
      const code = errorCodeOf(cause);
      if (code === 'INVALID_TAX_ID') {
        form.setError('taxId', { message: message(code) });
        return;
      }
      form.setError('root', { message: message(code) });
    }
  }

  if (loadError) return <Callout tone="danger">{loadError}</Callout>;
  if (!client) {
    return (
      <div className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted">
        <Spinner size="sm" />
        {t('loading')}
      </div>
    );
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        noValidate
        aria-label={t('title')}
        className="flex flex-col p-4 gap-y-4 border border-border rounded-xl"
      >
        <h3 className="text-heading-6">{t('title')}</h3>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="legalName"
            render={({ field }) => (
              <FormItem className="sm:col-span-2">
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
            control={form.control}
            name="taxId"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('taxId.label')}</FormLabel>
                <FormControl>
                  <Input
                    maxLength={TAX_ID_MAX_LENGTH}
                    inputMode="numeric"
                    placeholder={t('taxId.placeholder')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>{t('taxId.hint')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
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
            variant="outline"
            pending={form.formState.isSubmitting}
            pendingLabel={t('submitting')}
          >
            {t('submit')}
          </PendingButton>
        </div>
      </form>
    </Form>
  );
}
