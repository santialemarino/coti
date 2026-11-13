'use client';

import { PlusIcon, Trash2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';

import {
  Button,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  RowActionButton,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import type { PromotionValues } from '@/app/(protected)/settings/promotions/form-schema';
import { AmountInput } from '@/components/amount-input';

interface PromotionTiersProps {
  pending: boolean;
}

// The quantity bands of a tiered rule: each band gives its own benefit, both bounds inclusive.
export function PromotionTiers({ pending }: PromotionTiersProps) {
  const t = useTranslations('promotions.form.tiers');
  const form = useFormContext<PromotionValues>();
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'tiers' });
  const actionType = useWatch({ control: form.control, name: 'actionType' });
  const percentage = actionType === 'PERCENTAGE';

  return (
    <div className="flex flex-col gap-y-4">
      <p className="text-paragraph-sm text-foreground-muted">{t('hint')}</p>

      <ul className="flex flex-col gap-y-4">
        {fields.map((field, index) => (
          <li
            key={field.id}
            className="grid grid-cols-1 items-start gap-x-3 gap-y-3 sm:grid-cols-[1fr_1fr_1fr_auto]"
          >
            <FormField
              control={form.control}
              name={`tiers.${index}.fromQuantity`}
              render={({ field: from }) => (
                <FormItem>
                  <FormLabel required className={index === 0 ? undefined : 'sm:sr-only'}>
                    {t('from.label')}
                  </FormLabel>
                  <FormControl>
                    <AmountInput placeholder={t('from.placeholder')} {...from} disabled={pending} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`tiers.${index}.toQuantity`}
              render={({ field: to }) => (
                <FormItem>
                  <FormLabel className={index === 0 ? undefined : 'sm:sr-only'}>
                    {t('to.label')}
                  </FormLabel>
                  <FormControl>
                    <AmountInput placeholder={t('to.placeholder')} {...to} disabled={pending} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name={`tiers.${index}.value`}
              render={({ field: value }) => (
                <FormItem>
                  <FormLabel required className={index === 0 ? undefined : 'sm:sr-only'}>
                    {t(`value.${actionType}`)}
                  </FormLabel>
                  <FormControl>
                    <AmountInput
                      placeholder={t('value.placeholder')}
                      prefix={percentage ? undefined : '$'}
                      suffix={percentage ? '%' : undefined}
                      {...value}
                      disabled={pending}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div className={cn('sm:min-w-8', index === 0 && 'sm:pt-7')}>
              {fields.length > 1 ? (
                <RowActionButton
                  icon={Trash2Icon}
                  label={t('remove')}
                  tone="danger"
                  disabled={pending}
                  onClick={() => remove(index)}
                />
              ) : null}
            </div>
          </li>
        ))}
      </ul>

      <FormField
        control={form.control}
        name="tiers"
        render={() => (
          <FormItem>
            <FormMessage />
          </FormItem>
        )}
      />

      <div>
        <Button
          variant="outline"
          disabled={pending}
          onClick={() => append({ fromQuantity: '', toQuantity: '', value: '' })}
        >
          <PlusIcon aria-hidden="true" />
          {t('add')}
        </Button>
      </div>
    </div>
  );
}
