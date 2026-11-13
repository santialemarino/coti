'use client';

import { useId, useMemo, useTransition } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
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
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
  Switch,
  Textarea,
} from '@repo/ui/components';
import { PromotionTargets } from '@/app/(protected)/settings/promotions/_components/promotion-targets';
import { PromotionTiers } from '@/app/(protected)/settings/promotions/_components/promotion-tiers';
import { createPromotion, updatePromotion } from '@/app/(protected)/settings/promotions/actions';
import {
  allowsSpecialPrice,
  MIN_CONDITIONS,
  PROMOTION_DESCRIPTION_MAX_LENGTH,
  PROMOTION_NAME_MAX_LENGTH,
  promotionSchema,
  type PromotionValues,
} from '@/app/(protected)/settings/promotions/form-schema';
import { AmountInput } from '@/components/amount-input';
import { QuantityInput } from '@/components/quantity-input';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { usePendingRouter } from '@/hooks/use-pending-router';
import type { Branch } from '@/lib/api/branches';
import type { ProductFamily } from '@/lib/api/products';
import {
  PROMOTION_ACTION_TYPES,
  PROMOTION_CONDITION_TYPES,
  type PromotionConditionType,
} from '@/lib/constants/promotions';
import { FORM_VALIDATION } from '@/lib/forms/options';

// The scope option that stands for every branch; the API reads it as an absent branch.
const WHOLE_ACCOUNT = '';

interface PromotionFormProps {
  /* The rule being edited; absent when creating one. */
  promotionId?: string;
  initialValues: PromotionValues;
  families: ProductFamily[];
  /* The account's open branches. A rule may still name a closed one, which stays selectable. */
  branches: Branch[];
}

export function PromotionForm({
  promotionId,
  initialValues,
  families,
  branches,
}: PromotionFormProps) {
  const router = usePendingRouter();
  const t = useTranslations('promotions.form');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('promotions');
  const fieldId = useId();
  const schema = useMemo(() => promotionSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const [saving, startSave] = useTransition();
  const form = useForm<PromotionValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: initialValues,
  });
  const conditionType = useWatch({ control: form.control, name: 'conditionType' });
  const actionType = useWatch({ control: form.control, name: 'actionType' });
  const tiered = conditionType === 'QUANTITY_TIERED';
  const onTotal = conditionType === 'ON_TOTAL';
  const percentage = actionType === 'PERCENTAGE';
  const editing = promotionId !== undefined;

  const scopeOptions = [
    { value: WHOLE_ACCOUNT, label: t('scope.account') },
    ...branches.map((branch) => ({ value: branch.id, label: branch.name })),
  ];
  // With one branch "every branch" and "this branch" are the same rule, so there is no choice.
  const showScope = branches.length > 1 || initialValues.branchId !== WHOLE_ACCOUNT;
  const actionTypes = PROMOTION_ACTION_TYPES.filter(
    (type) => type !== 'SPECIAL_PRICE' || allowsSpecialPrice(conditionType),
  );

  /*
   * A type change keeps everything typed and only tops up what the new type requires, so moving
   * between two types and back loses nothing.
   */
  function onConditionTypeChange(next: PromotionConditionType) {
    form.setValue('conditionType', next);
    if (form.getValues('actionType') === 'SPECIAL_PRICE' && !allowsSpecialPrice(next)) {
      form.setValue('actionType', 'PERCENTAGE');
    }
    const conditions = form.getValues('conditions');
    const missing = MIN_CONDITIONS[next] - conditions.length;
    if (missing > 0) {
      form.setValue('conditions', [
        ...conditions,
        ...Array.from({ length: missing }, () => ({
          kind: 'product' as const,
          targetId: '',
          label: '',
          minQuantity: '',
        })),
      ]);
    }
    if (next === 'QUANTITY_TIERED' && form.getValues('tiers').length === 0) {
      form.setValue('tiers', [{ fromQuantity: '', toQuantity: '', value: '' }]);
    }
    if (form.formState.isSubmitted) void form.trigger();
  }

  function onSubmit(values: PromotionValues) {
    startSave(async () => {
      const result =
        promotionId !== undefined
          ? await updatePromotion(promotionId, values)
          : await createPromotion(values);
      if (!result.ok) {
        form.setError('root', { message: message(result.error) });
        return;
      }
      toast.success(t(editing ? 'toast.updated' : 'toast.created', { name: values.name }));
      router.push(ROUTES.promotionSettings);
    });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-6">
        <Card>
          <CardHeader>
            <CardTitle>{t('sections.details.title')}</CardTitle>
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
                      maxLength={PROMOTION_NAME_MAX_LENGTH}
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
                      maxLength={PROMOTION_DESCRIPTION_MAX_LENGTH}
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
            {showScope ? (
              <FormField
                control={form.control}
                name="branchId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('scope.label')}</FormLabel>
                    <FormControl>
                      <Combobox
                        className="w-full sm:w-80"
                        options={scopeOptions}
                        value={field.value}
                        placeholder={t('scope.placeholder')}
                        disabled={saving}
                        onValueChange={field.onChange}
                      />
                    </FormControl>
                    <FormDescription>{t('scope.hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('sections.rule.title')}</CardTitle>
            <CardDescription>{t('sections.rule.description')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-y-6">
            <div className="flex flex-col gap-y-3">
              <Label id={`${fieldId}-condition`} required>
                {t('conditionType.label')}
              </Label>
              <RadioGroup
                aria-labelledby={`${fieldId}-condition`}
                className="grid grid-cols-1 gap-3 sm:grid-cols-2"
                value={conditionType}
                onValueChange={(value) => onConditionTypeChange(value as PromotionConditionType)}
                disabled={saving}
              >
                {PROMOTION_CONDITION_TYPES.map((type) => (
                  <div
                    key={type}
                    className="flex items-start gap-x-2.5 p-3 border border-border rounded-lg"
                  >
                    <RadioGroupItem
                      id={`${fieldId}-condition-${type}`}
                      value={type}
                      className="mt-0.5"
                    />
                    <Label
                      htmlFor={`${fieldId}-condition-${type}`}
                      className="flex flex-col items-start gap-y-0.5 cursor-pointer"
                    >
                      <span className="text-paragraph-sm-medium text-foreground">
                        {t(`conditionType.${type}.label`)}
                      </span>
                      <span className="text-paragraph-xs text-foreground-muted">
                        {t(`conditionType.${type}.description`)}
                      </span>
                    </Label>
                  </div>
                ))}
              </RadioGroup>
            </div>

            <FormField
              control={form.control}
              name="actionType"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('actionType.label')}</FormLabel>
                  <FormControl>
                    <Combobox
                      className="w-full sm:w-80"
                      options={actionTypes.map((type) => ({
                        value: type,
                        label: t(`actionType.${type}`),
                      }))}
                      value={field.value}
                      placeholder={t('actionType.placeholder')}
                      disabled={saving}
                      onValueChange={field.onChange}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {tiered ? null : (
              <FormField
                control={form.control}
                name="actionValue"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t(`actionValue.${actionType}`)}</FormLabel>
                    <FormControl>
                      <AmountInput
                        className="sm:w-48"
                        placeholder={t('actionValue.placeholder')}
                        prefix={percentage ? undefined : '$'}
                        suffix={percentage ? '%' : undefined}
                        {...field}
                        disabled={saving}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {onTotal ? (
              <FormField
                control={form.control}
                name="minTotal"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('minTotal.label')}</FormLabel>
                    <FormControl>
                      <AmountInput
                        className="sm:w-48"
                        placeholder={t('minTotal.placeholder')}
                        prefix="$"
                        {...field}
                        disabled={saving}
                      />
                    </FormControl>
                    <FormDescription>{t('minTotal.hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}
          </CardContent>
        </Card>

        {onTotal ? null : (
          <Card>
            <CardHeader>
              <CardTitle>{t('sections.targets.title')}</CardTitle>
            </CardHeader>
            <CardContent>
              <PromotionTargets families={families} pending={saving} />
            </CardContent>
          </Card>
        )}

        {tiered ? (
          <Card>
            <CardHeader>
              <CardTitle>{t('sections.tiers.title')}</CardTitle>
            </CardHeader>
            <CardContent>
              <PromotionTiers pending={saving} />
            </CardContent>
          </Card>
        ) : null}

        <Card>
          <CardHeader>
            <CardTitle>{t('sections.validity.title')}</CardTitle>
            <CardDescription>{t('sections.validity.description')}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-y-5">
            <div className="grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2">
              <FormField
                control={form.control}
                name="validFrom"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('validFrom.label')}</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} disabled={saving} />
                    </FormControl>
                    <FormDescription>{t('validFrom.hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="validTo"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('validTo.label')}</FormLabel>
                    <FormControl>
                      <Input type="date" {...field} disabled={saving} />
                    </FormControl>
                    <FormDescription>{t('validTo.hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <FormField
              control={form.control}
              name="priority"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('priority.label')}</FormLabel>
                  <FormControl>
                    <QuantityInput
                      className="sm:w-32"
                      placeholder={t('priority.placeholder')}
                      {...field}
                      disabled={saving}
                    />
                  </FormControl>
                  <FormDescription>{t('priority.hint')}</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="isExclusive"
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
                    <FormLabel>{t('isExclusive.label')}</FormLabel>
                    <FormDescription>{t('isExclusive.hint')}</FormDescription>
                  </div>
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

        <FormRootMessage />

        <div className="flex flex-col-reverse gap-y-3 sm:flex-row sm:justify-end sm:gap-x-3">
          <Button asChild variant="outline">
            <Link href={ROUTES.promotionSettings}>{t('cancel')}</Link>
          </Button>
          <PendingButton
            type="submit"
            pending={saving}
            pendingLabel={t(editing ? 'submit.updating' : 'submit.creating')}
          >
            {t(editing ? 'submit.update' : 'submit.create')}
          </PendingButton>
        </div>
      </form>
    </Form>
  );
}
