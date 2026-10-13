'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';
import { z } from 'zod';

import { Form, FormRootMessage, PendingButton } from '@repo/ui/components';
import { updatePointsOfSale } from '@/app/(protected)/settings/invoicing/actions';
import { PointsOfSaleTable } from '@/components/points-of-sale-table';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { InvoicingBranch } from '@/lib/api/invoicing';
import { FORM_VALIDATION } from '@/lib/forms/options';
import {
  pointsOfSaleSchema,
  pointsOfSaleValues,
  type PointOfSaleValue,
} from '@/lib/forms/points-of-sale';

interface PointsOfSaleFormValues {
  branches: PointOfSaleValue[];
}

interface PointsOfSaleFormProps {
  branches: InvoicingBranch[];
  disabled?: boolean;
}

// The points of sale on their own, saved without touching the rest of the invoicing settings.
export function PointsOfSaleForm({ branches, disabled = false }: PointsOfSaleFormProps) {
  const router = useRouter();
  const t = useTranslations('invoicing.settings');
  const tShared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('invoicing.settings');
  const schema = useMemo(
    () => z.object({ branches: pointsOfSaleSchema({ field: t, shared: tShared }) }),
    [t, tShared],
  );
  const form = useForm<PointsOfSaleFormValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { branches: pointsOfSaleValues(branches) },
  });
  const pending = form.formState.isSubmitting;

  async function onSubmit(values: PointsOfSaleFormValues) {
    const result = await updatePointsOfSale(values.branches);
    if (!result.settings) {
      form.setError('root', { message: message(result.error) });
      return;
    }
    toast.success(t('pointsOfSale.saved'));
    form.reset({ branches: pointsOfSaleValues(result.settings.branches) });
    router.refresh();
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-4">
        <PointsOfSaleTable
          control={form.control}
          branches={branches}
          disabled={disabled || pending}
        />
        <FormRootMessage />
        <PendingButton
          type="submit"
          className="self-start"
          pending={pending}
          disabled={disabled}
          pendingLabel={t('submitting')}
        >
          {t('pointsOfSale.save')}
        </PendingButton>
      </form>
    </Form>
  );
}
