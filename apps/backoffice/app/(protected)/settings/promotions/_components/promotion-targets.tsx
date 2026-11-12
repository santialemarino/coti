'use client';

import { useState } from 'react';
import { PackageSearchIcon, PlusIcon, Trash2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useFieldArray, useFormContext, useWatch } from 'react-hook-form';

import {
  Button,
  Combobox,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Label,
  RowActionButton,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import {
  CONDITION_TARGET_KINDS,
  MIN_CONDITIONS,
  type ConditionTargetKind,
  type PromotionValues,
} from '@/app/(protected)/settings/promotions/form-schema';
import { AmountInput } from '@/components/amount-input';
import { ProductSearchDialog } from '@/components/product-search-dialog';
import type { CatalogProduct } from '@/lib/api/catalog';
import type { ProductFamily } from '@/lib/api/products';

interface PromotionTargetsProps {
  families: ProductFamily[];
  pending: boolean;
}

/*
 * What a rule applies to: products, families or subgroups, each with an optional minimum quantity.
 * A family or a subgroup is picked from the taxonomy; a product is searched in the catalog.
 */
export function PromotionTargets({ families, pending }: PromotionTargetsProps) {
  const t = useTranslations('promotions.form.conditions');
  const form = useFormContext<PromotionValues>();
  const { fields, append, remove, update } = useFieldArray({
    control: form.control,
    name: 'conditions',
  });
  const conditionType = useWatch({ control: form.control, name: 'conditionType' });
  const conditions = useWatch({ control: form.control, name: 'conditions' });
  const [pickingIndex, setPickingIndex] = useState<number | null>(null);
  const tiered = conditionType === 'QUANTITY_TIERED';
  const minimum = MIN_CONDITIONS[conditionType];

  const familyOptions = families.map((family) => ({ value: family.id, label: family.name }));
  const subgroupOptions = families.flatMap((family) =>
    family.subgroups.map((subgroup) => ({
      value: subgroup.id,
      label: subgroup.name,
      group: family.name,
    })),
  );
  const kindOptions = CONDITION_TARGET_KINDS.map((kind) => ({
    value: kind,
    label: t(`kind.${kind}`),
  }));

  function onKindChange(index: number, kind: ConditionTargetKind) {
    const current = conditions[index];
    if (!current || current.kind === kind) return;
    update(index, { kind, targetId: '', label: '', minQuantity: current.minQuantity });
  }

  function onProductSelected(product: CatalogProduct) {
    if (pickingIndex === null) return;
    form.setValue(`conditions.${pickingIndex}.label`, product.name);
    form.setValue(`conditions.${pickingIndex}.targetId`, product.id, {
      shouldValidate: form.formState.isSubmitted,
    });
    setPickingIndex(null);
  }

  return (
    <div className="flex flex-col gap-y-4">
      <p className="text-paragraph-sm text-foreground-muted">
        {t(`hint.${conditionType === 'ITEM_SET' ? 'itemSet' : 'perItem'}`, { count: minimum })}
      </p>

      <ul className="flex flex-col gap-y-4">
        {fields.map((field, index) => {
          const kind = conditions[index]?.kind ?? 'product';
          return (
            <li
              key={field.id}
              className="grid grid-cols-1 items-start gap-x-3 gap-y-3 sm:grid-cols-[10rem_1fr_9rem_auto]"
            >
              <div className="flex flex-col gap-y-2">
                <Label
                  htmlFor={`${field.id}-kind`}
                  className={index === 0 ? undefined : 'sm:sr-only'}
                >
                  {t('kind.label')}
                </Label>
                <Combobox
                  id={`${field.id}-kind`}
                  options={kindOptions}
                  value={kind}
                  placeholder={t('kind.placeholder')}
                  disabled={pending}
                  onValueChange={(value) => onKindChange(index, value as ConditionTargetKind)}
                />
              </div>

              <FormField
                control={form.control}
                name={`conditions.${index}.targetId`}
                render={({ field: target }) => (
                  <FormItem>
                    <FormLabel required className={index === 0 ? undefined : 'sm:sr-only'}>
                      {t('target.label')}
                    </FormLabel>
                    <FormControl>
                      {kind === 'product' ? (
                        <Button
                          variant="outline"
                          className="w-full justify-start"
                          disabled={pending}
                          onClick={() => setPickingIndex(index)}
                        >
                          <PackageSearchIcon aria-hidden="true" />
                          <span className="truncate">
                            {conditions[index]?.label ||
                              (target.value ? t('target.unknownProduct') : t('target.pickProduct'))}
                          </span>
                        </Button>
                      ) : (
                        <Combobox
                          searchable
                          options={kind === 'family' ? familyOptions : subgroupOptions}
                          value={target.value || null}
                          placeholder={t(
                            kind === 'family' ? 'target.pickFamily' : 'target.pickSubgroup',
                          )}
                          searchPlaceholder={t('target.search')}
                          emptyLabel={t('target.empty')}
                          disabled={pending}
                          onValueChange={target.onChange}
                        />
                      )}
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {tiered ? (
                <div aria-hidden="true" className="hidden sm:block" />
              ) : (
                <FormField
                  control={form.control}
                  name={`conditions.${index}.minQuantity`}
                  render={({ field: quantity }) => (
                    <FormItem>
                      <FormLabel className={index === 0 ? undefined : 'sm:sr-only'}>
                        {t('minQuantity.label')}
                      </FormLabel>
                      <FormControl>
                        <AmountInput
                          placeholder={t('minQuantity.placeholder')}
                          {...quantity}
                          disabled={pending}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              {/* At the minimum the hint above already says why a target cannot go. */}
              <div className={cn('sm:min-w-8', index === 0 && 'sm:pt-7')}>
                {fields.length > Math.max(minimum, 1) ? (
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
          );
        })}
      </ul>

      <FormField
        control={form.control}
        name="conditions"
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
          onClick={() => append({ kind: 'product', targetId: '', label: '', minQuantity: '' })}
        >
          <PlusIcon aria-hidden="true" />
          {t('add')}
        </Button>
      </div>

      <ProductSearchDialog
        open={pickingIndex !== null}
        onOpenChange={(open) => !open && setPickingIndex(null)}
        title={t('target.dialogTitle')}
        excludedProductIds={conditions
          .filter((condition, index) => condition.kind === 'product' && index !== pickingIndex)
          .map((condition) => condition.targetId)
          .filter(Boolean)}
        onSelect={onProductSelected}
      />
    </div>
  );
}
