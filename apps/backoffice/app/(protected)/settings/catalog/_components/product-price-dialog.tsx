'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
  Combobox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  PendingButton,
  Spinner,
} from '@repo/ui/components';
import { getProductPrice, setProductPrice } from '@/app/(protected)/settings/catalog/actions';
import {
  productPriceSchema,
  type ProductPriceValues,
} from '@/app/(protected)/settings/catalog/price-schema';
import { AmountInput } from '@/components/amount-input';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { errorCodeOf } from '@/lib/api/errors';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface ProductPriceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  productId: string;
  productName: string;
  branches: Branch[];
  initialBranchId?: string;
}

export function ProductPriceDialog({
  open,
  onOpenChange,
  productId,
  productName,
  branches,
  initialBranchId,
}: ProductPriceDialogProps) {
  const t = useTranslations('products');
  const tShared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('products');
  const schema = useMemo(() => productPriceSchema({ field: t, shared: tShared }), [t, tShared]);
  const [loading, setLoading] = useState(false);
  const [loadFailed, setLoadFailed] = useState(false);
  const [loadAttempt, setLoadAttempt] = useState(0);
  const [loadedPriceKey, setLoadedPriceKey] = useState<string | null>(null);
  const saving = useRef(false);
  const form = useForm<ProductPriceValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { branchId: '', price: '', minPrice: '', currency: 'ARS' },
  });
  const branchId = form.watch('branchId');
  const activeBranches = branches.filter((branch) => branch.isActive);
  const branchName = activeBranches.find((branch) => branch.id === branchId)?.name ?? '';
  const pending = form.formState.isSubmitting;
  const ready =
    !!branchId && loadedPriceKey === `${productId}/${branchId}` && !loading && !loadFailed;

  useEffect(() => {
    if (!open) return;
    setLoadedPriceKey(null);
    const selected = branches.find((branch) => branch.id === initialBranchId && branch.isActive);
    const soleBranch = branches.filter((branch) => branch.isActive);
    form.reset({
      branchId: selected?.id ?? (soleBranch.length === 1 ? soleBranch[0]!.id : ''),
      price: '',
      minPrice: '',
      currency: 'ARS',
    });
  }, [open, productId, initialBranchId, branches, form]);

  useEffect(() => {
    if (!open || !branchId) return;
    let cancelled = false;
    setLoading(true);
    setLoadFailed(false);
    form.clearErrors();
    void getProductPrice(productId, branchId)
      .then((result) => {
        if (cancelled) return;
        if (!result.ok) {
          setLoadFailed(true);
          form.setError('root', { message: message(result.error) });
        } else {
          setLoadedPriceKey(`${productId}/${branchId}`);
          form.reset({
            branchId,
            price: result.current?.price ?? '',
            minPrice: result.current?.minPrice ?? '',
            currency: result.current?.currency ?? 'ARS',
          });
        }
        setLoading(false);
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setLoadFailed(true);
        setLoading(false);
        form.setError('root', { message: message(errorCodeOf(error)) });
      });
    return () => {
      cancelled = true;
    };
  }, [open, productId, branchId, loadAttempt, form, message]);

  async function onSubmit(values: ProductPriceValues) {
    if (saving.current || !ready) return;
    saving.current = true;
    try {
      const result = await setProductPrice(productId, values);
      if (!result.ok) {
        form.setError('root', { message: message(result.error) });
        return;
      }
      toast.success(t('priceEditor.saved', { name: productName, branch: branchName }));
      onOpenChange(false);
    } catch (error) {
      form.setError('root', { message: message(errorCodeOf(error)) });
    } finally {
      saving.current = false;
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !pending && onOpenChange(next)}>
      <DialogContent
        closeOnClickOutside={!pending && !form.formState.isDirty}
        className="sm:max-w-lg"
      >
        <DialogHeader>
          <DialogTitle>{t('priceEditor.title', { name: productName })}</DialogTitle>
          <DialogDescription>{t('priceEditor.description')}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form noValidate onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-y-5">
            <FormField
              control={form.control}
              name="branchId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('priceEditor.branch')}</FormLabel>
                  <FormControl>
                    {activeBranches.length === 0 ? (
                      <p className="text-paragraph-sm text-foreground-muted">
                        {t('priceEditor.noBranches')}
                      </p>
                    ) : activeBranches.length === 1 ? (
                      <p className="text-paragraph-sm text-foreground">{activeBranches[0]!.name}</p>
                    ) : (
                      <Combobox
                        ref={field.ref}
                        value={field.value || null}
                        onValueChange={field.onChange}
                        disabled={pending}
                        placeholder={t('priceEditor.branchPlaceholder')}
                        options={activeBranches.map((branch) => ({
                          value: branch.id,
                          label: branch.name,
                        }))}
                      />
                    )}
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            {loading ? (
              <div className="flex min-h-28 items-center justify-center">
                <Spinner size="sm" />
              </div>
            ) : (
              <div className="grid min-h-28 content-start gap-4 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="price"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel required>{t('price.label')}</FormLabel>
                      <FormControl>
                        <AmountInput
                          prefix={form.getValues('currency')}
                          value={field.value}
                          onChange={field.onChange}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          disabled={!ready || pending}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name="minPrice"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('minPrice.label')}</FormLabel>
                      <FormControl>
                        <AmountInput
                          prefix={form.getValues('currency')}
                          value={field.value}
                          onChange={field.onChange}
                          onBlur={field.onBlur}
                          ref={field.ref}
                          disabled={!ready || pending}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            )}
            <FormRootMessage />
            {loadFailed ? (
              <Button variant="outline" onClick={() => setLoadAttempt((attempt) => attempt + 1)}>
                {t('priceEditor.retry')}
              </Button>
            ) : null}
            <DialogFooter>
              <Button variant="outline" disabled={pending} onClick={() => onOpenChange(false)}>
                {t('cancel')}
              </Button>
              <PendingButton
                type="submit"
                pending={pending}
                disabled={!ready}
                pendingLabel={t('priceEditor.saving')}
              >
                {t('priceEditor.save')}
              </PendingButton>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
