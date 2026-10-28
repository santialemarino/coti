'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  PendingButton,
  TaxIdInput,
} from '@repo/ui/components';
import { isValidCuit } from '@repo/ui/lib';
import { listRecoveryContacts, type RecoveryContact } from '@/app/forgot-password/actions';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface RecoveryIdentifyStepProps {
  // Set when the caller comes back to this step, never on the first render.
  autoFocus?: boolean;
  onFound: (taxId: string, contacts: RecoveryContact[]) => void;
}

export function RecoveryIdentifyStep({ autoFocus, onFound }: RecoveryIdentifyStepProps) {
  const t = useTranslations('customerRecovery');
  const auth = useTranslations('customerAuth');
  const form = useForm({
    ...FORM_VALIDATION,
    resolver: zodResolver(
      z.object({
        taxId: z
          .string()
          .refine(
            (value) => isValidCuit(value) && value !== '00000000000',
            auth('validation.taxInvalid'),
          ),
      }),
    ),
    defaultValues: { taxId: '' },
  });

  async function submit(values: { taxId: string }) {
    const result = await listRecoveryContacts(values.taxId);
    if (result.error || !result.contacts) {
      form.setError('root', { message: t(`errors.${result.error ?? 'unavailable'}`) });
      return;
    }
    onFound(values.taxId, result.contacts);
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} noValidate className="flex flex-col gap-y-5">
        <p className="text-paragraph-sm text-foreground-muted">{t('identify')}</p>
        <FormField
          control={form.control}
          name="taxId"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{auth('fields.taxId')}</FormLabel>
              <FormControl>
                <TaxIdInput kind="cuit" {...field} autoComplete="username" autoFocus={autoFocus} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormRootMessage />
        <PendingButton
          type="submit"
          pending={form.formState.isSubmitting}
          pendingLabel={t('searching')}
        >
          {t('continue')}
        </PendingButton>
      </form>
    </Form>
  );
}
