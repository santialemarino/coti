'use client';

import { useId, useMemo, useState, useTransition } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { PackageSearchIcon, PlusIcon, Trash2Icon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useFieldArray, useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Checkbox,
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
  Label,
  PendingButton,
  RowActionButton,
  Switch,
  Textarea,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { createCombo, updateCombo } from '@/app/(protected)/settings/combos/actions';
import {
  COMBO_DESCRIPTION_MAX_LENGTH,
  COMBO_MIN_ITEMS,
  COMBO_NAME_MAX_LENGTH,
  comboSchema,
  type ComboValues,
} from '@/app/(protected)/settings/combos/form-schema';
import { AmountInput } from '@/components/amount-input';
import { ProductSearchDialog } from '@/components/product-search-dialog';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { usePendingRouter } from '@/hooks/use-pending-router';
import type { Branch } from '@/lib/api/branches';
import type { CatalogProduct } from '@/lib/api/catalog';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ComboFormProps {
  /* The combo being edited; absent when creating one. */
  comboId?: string;
  initialValues: ComboValues;
  branches: Branch[];
}

export function ComboForm({ comboId, initialValues, branches }: ComboFormProps) {
  const router = usePendingRouter();
  const t = useTranslations('combos.form');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('combos');
  const fieldId = useId();
  const schema = useMemo(() => comboSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [saving, startSave] = useTransition();
  const [pickingIndex, setPickingIndex] = useState<number | null>(null);
  const form = useForm<ComboValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: initialValues,
  });
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'items' });
  const items = useWatch({ control: form.control, name: 'items' });
  const editing = comboId !== undefined;
  // With one branch there is nothing to choose: the combo is sold there.
  const showBranches = branches.length > 1;

  function onProductSelected(product: CatalogProduct) {
    if (pickingIndex === null) return;
    form.setValue(`items.${pickingIndex}.label`, product.name);
    form.setValue(`items.${pickingIndex}.productId`, product.id, {
      shouldValidate: form.formState.isSubmitted,
    });
    setPickingIndex(null);
  }

  function onSubmit(values: ComboValues) {
    startSave(async () => {
      const result =
        comboId !== undefined ? await updateCombo(comboId, values) : await createCombo(values);
      if (!result.ok) {
        form.setError('root', { message: message(result.error) });
        return;
      }
      toast.success(t(editing ? 'toast.updated' : 'toast.created', { name: values.name }));
      router.push(ROUTES.comboSettings);
    });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t('sections.details')}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-y-5">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('name.label')}</FormLabel>
                  <FormControl>
                    <Input
                      maxLength={COMBO_NAME_MAX_LENGTH}
                      placeholder={t('name.placeholder')}
                      {...field}
                      disabled={saving}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('description.label')}</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={2}
                      maxLength={COMBO_DESCRIPTION_MAX_LENGTH}
                      placeholder={t('description.placeholder')}
                      {...field}
                      disabled={saving}
                    />
                  </FormControl>
                  <FormDescription>{t('description.hint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name="isActive"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start gap-x-3">
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      disabled={saving}
                    />
                  </FormControl>
                  <div className="flex flex-col gap-y-1">
                    <FormLabel>{t('isActive.label')}</FormLabel>
                    <FormDescription>{t('isActive.hint')}</FormDescription>
                  </div>
                </FormItem>
              )}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('sections.products')}</CardTitle>
            <CardDescription>{t('items.hint', { count: COMBO_MIN_ITEMS })}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-y-4">
            <ul className="flex flex-col gap-y-4">
              {fields.map((field, index) => (
                <li
                  key={field.id}
                  className="grid grid-cols-1 items-start gap-x-3 gap-y-3 sm:grid-cols-[1fr_9rem_auto]"
                >
                  <FormField
                    control={form.control}
                    name={`items.${index}.productId`}
                    render={({ field: product }) => (
                      <FormItem>
                        <FormLabel required className={index === 0 ? undefined : 'sm:sr-only'}>
                          {t('items.product')}
                        </FormLabel>
                        <FormControl>
                          <Button
                            variant="outline"
                            className="w-full justify-start"
                            disabled={saving}
                            onClick={() => setPickingIndex(index)}
                          >
                            <PackageSearchIcon aria-hidden="true" />
                            <span className="truncate">
                              {items[index]?.label ||
                                (product.value
                                  ? t('items.unknownProduct')
                                  : t('items.pickProduct'))}
                            </span>
                          </Button>
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={form.control}
                    name={`items.${index}.quantity`}
                    render={({ field: quantity }) => (
                      <FormItem>
                        <FormLabel required className={index === 0 ? undefined : 'sm:sr-only'}>
                          {t('items.quantity')}
                        </FormLabel>
                        <FormControl>
                          <AmountInput
                            placeholder={t('items.quantityPlaceholder')}
                            {...quantity}
                            disabled={saving}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  {/* At the minimum the hint above already says why a product cannot go. */}
                  <div className={cn('sm:min-w-8', index === 0 && 'sm:pt-7')}>
                    {fields.length > COMBO_MIN_ITEMS ? (
                      <RowActionButton
                        icon={Trash2Icon}
                        label={t('items.remove')}
                        tone="danger"
                        disabled={saving}
                        onClick={() => remove(index)}
                      />
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
            <FormField
              control={form.control}
              name="items"
              render={() => (
                <FormItem>
                  <FormMessage />
                </FormItem>
              )}
            />
            <div>
              <Button
                variant="outline"
                disabled={saving}
                onClick={() => append({ productId: '', label: '', quantity: '1' })}
              >
                <PlusIcon aria-hidden="true" />
                {t('items.add')}
              </Button>
            </div>
            <p className="text-paragraph-xs text-foreground-muted">
              {t('discountHint')}{' '}
              <InlineLink asChild>
                <Link href={ROUTES.promotionNew}>{t('discountLink')}</Link>
              </InlineLink>
            </p>
          </CardContent>
        </Card>

        {showBranches ? (
          <Card>
            <CardHeader>
              <CardTitle id={`${fieldId}-branches`}>{t('sections.branches')}</CardTitle>
              <CardDescription>{t('branches.hint')}</CardDescription>
            </CardHeader>
            <CardContent>
              <FormField
                control={form.control}
                name="branchIds"
                render={({ field }) => (
                  <FormItem>
                    <div
                      role="group"
                      aria-labelledby={`${fieldId}-branches`}
                      className="flex flex-col gap-y-3"
                    >
                      {branches.map((branch) => {
                        const checked = field.value.includes(branch.id);
                        return (
                          <div key={branch.id} className="flex items-center gap-x-2.5">
                            <Checkbox
                              id={`${fieldId}-branch-${branch.id}`}
                              checked={checked}
                              disabled={saving}
                              onCheckedChange={(next) =>
                                field.onChange(
                                  next === true
                                    ? [...field.value, branch.id]
                                    : field.value.filter((id) => id !== branch.id),
                                )
                              }
                            />
                            <Label
                              htmlFor={`${fieldId}-branch-${branch.id}`}
                              className="cursor-pointer"
                            >
                              {branch.name}
                            </Label>
                          </div>
                        );
                      })}
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </CardContent>
          </Card>
        ) : null}

        <FormRootMessage />

        <div className="flex flex-col-reverse gap-y-3 sm:flex-row sm:justify-end sm:gap-x-3">
          <Button asChild variant="outline">
            <Link href={ROUTES.comboSettings}>{t('cancel')}</Link>
          </Button>
          <PendingButton
            type="submit"
            pending={saving}
            pendingLabel={t(editing ? 'submit.updating' : 'submit.creating')}
          >
            {t(editing ? 'submit.update' : 'submit.create')}
          </PendingButton>
        </div>

        <ProductSearchDialog
          open={pickingIndex !== null}
          onOpenChange={(open) => !open && setPickingIndex(null)}
          title={t('items.dialogTitle')}
          excludedProductIds={items
            .filter((_, index) => index !== pickingIndex)
            .map((item) => item.productId)
            .filter(Boolean)}
          onSelect={onProductSelected}
        />
      </form>
    </Form>
  );
}
