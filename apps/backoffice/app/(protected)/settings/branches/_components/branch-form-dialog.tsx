'use client';

import { useEffect, useMemo, useRef } from 'react';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Input,
  PendingButton,
} from '@repo/ui/components';
import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import type { Branch } from '@/lib/api/branches';
import { DEFAULT_EXPIRY_DAYS, EXPIRY_MAX_DAYS, EXPIRY_MIN_DAYS } from '@/lib/constants/branch';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface BranchFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /* Which copy the dialog wears. Explicit, because a null branch is what opening one looks like. */
  mode: 'create' | 'edit';
  /* The branch being edited. May go null while the dialog animates out. */
  branch: Branch | null;
  onSubmit: (values: BranchValues) => void;
  pending: boolean;
}

/*
 * One dialog for both opening and editing a branch: the fields, their validation and the request
 * body are identical, and only the copy and the target differ.
 *
 * The mode is tracked while open and held afterwards, the way `ConfirmDialog` holds its entity, so
 * a caller that clears its selection on close does not relabel the dialog mid-exit.
 */
export function BranchFormDialog({
  open,
  onOpenChange,
  mode,
  branch,
  onSubmit,
  pending,
}: BranchFormDialogProps) {
  const t = useTranslations('branches');
  const tErrors = useTranslations('common.form.errors');
  const schema = useMemo(() => branchSchema({ field: t, shared: tErrors }), [t, tErrors]);
  const lastMode = useRef(mode);
  if (open) lastMode.current = mode;
  const copy = open ? mode : lastMode.current;

  const form = useForm<BranchValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { name: '', address: '', defaultExpiryDays: String(DEFAULT_EXPIRY_DAYS) },
  });

  /*
   * Reset on open, not on mount: the dialog outlives every branch it edits, so without this the
   * second row opened would still be showing the first row's values.
   */
  useEffect(() => {
    if (!open) return;
    form.reset({
      name: branch?.name ?? '',
      address: branch?.address ?? '',
      defaultExpiryDays: String(branch?.defaultExpiryDays ?? DEFAULT_EXPIRY_DAYS),
    });
  }, [open, branch, form]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{t(`${copy}.title`)}</DialogTitle>
          <DialogDescription>{t(`${copy}.description`)}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-5">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('name.label')}</FormLabel>
                  <FormControl>
                    <Input
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('name.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="address"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('address.label')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete="street-address"
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('address.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="defaultExpiryDays"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('defaultExpiryDays.label')}</FormLabel>
                  <FormControl>
                    <Input
                      type="number"
                      inputMode="numeric"
                      min={EXPIRY_MIN_DAYS}
                      max={EXPIRY_MAX_DAYS}
                      placeholder={String(DEFAULT_EXPIRY_DAYS)}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('defaultExpiryDays.hint', { count: DEFAULT_EXPIRY_DAYS })}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => onOpenChange(false)}
              >
                {t('cancel')}
              </Button>
              <PendingButton type="submit" pending={pending} pendingLabel={t(`${copy}.submitting`)}>
                {t(`${copy}.submit`)}
              </PendingButton>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
