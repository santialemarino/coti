'use client';

import { useLayoutEffect, useMemo, useRef } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

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
  FormRootMessage,
  InlineLink,
  Input,
  PendingButton,
} from '@repo/ui/components';
import {
  arcaPointOfSaleSchema,
  type ARCAPointOfSaleValues,
} from '@/app/(protected)/settings/arca/form-schema';
import { verifyBranchPointOfSale } from '@/app/(protected)/settings/branches/actions';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ARCASetup } from '@/lib/api/arca-setup';
import type { Branch } from '@/lib/api/branches';
import { formatCuit } from '@/lib/api/invoicing';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface BranchPointOfSaleDialogProps {
  branch: Branch | null;
  setup: ARCASetup;
  companyTaxId: string | null;
  pointOfSale: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function BranchPointOfSaleDialog({
  branch,
  setup,
  companyTaxId,
  pointOfSale,
  open,
  onOpenChange,
}: BranchPointOfSaleDialogProps) {
  const t = useTranslations('branches.billing');
  const arca = useTranslations('arca');
  const shared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('arca');
  const router = useRouter();
  const schema = useMemo(() => arcaPointOfSaleSchema({ field: arca, shared }), [arca, shared]);
  const heldBranch = useRef(branch);
  if (branch) heldBranch.current = branch;
  const shown = branch ?? heldBranch.current;
  const form = useForm<ARCAPointOfSaleValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { pointOfSale: '' },
  });
  const pending = form.formState.isSubmitting;
  const ready = setup.enabled && setup.hasCertificate;

  useLayoutEffect(() => {
    if (open) form.reset({ pointOfSale: pointOfSale === null ? '' : String(pointOfSale) });
  }, [open, branch?.id, pointOfSale, form]);

  async function submit(values: ARCAPointOfSaleValues) {
    if (!branch) return;
    const result = await verifyBranchPointOfSale(values, branch.id);
    if (result.error) {
      form.setError('root', { message: message(result.error) });
      return;
    }
    if (!result.connection?.verified) {
      form.setError('root', {
        message: arca(`failures.${result.connection?.failure || 'UNAVAILABLE'}`),
      });
      return;
    }
    toast.success(t('saved', { branch: branch.name }));
    router.refresh();
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !pending && onOpenChange(next)}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{t('title', { branch: shown?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-y-1">
          <span className="text-paragraph-xs text-foreground-muted">{t('cuit')}</span>
          <span className="text-paragraph-sm-medium">
            {companyTaxId ? formatCuit(companyTaxId) : t('unset')}
          </span>
          <InlineLink href={ROUTES.accountSettings}>{t('account')}</InlineLink>
        </div>
        {ready ? (
          <Form {...form}>
            <form noValidate onSubmit={form.handleSubmit(submit)} className="flex flex-col gap-y-5">
              <FormField
                control={form.control}
                name="pointOfSale"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('pointOfSale')}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        inputMode="numeric"
                        maxLength={5}
                        disabled={pending}
                        placeholder={arca('pointOfSale.placeholder')}
                      />
                    </FormControl>
                    <FormDescription>{arca('pointOfSale.hint')}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormRootMessage />
              <DialogFooter>
                <Button variant="outline" disabled={pending} onClick={() => onOpenChange(false)}>
                  {arca('cancel')}
                </Button>
                <PendingButton type="submit" pending={pending} pendingLabel={arca('verifying')}>
                  {t('submit')}
                </PendingButton>
              </DialogFooter>
            </form>
          </Form>
        ) : (
          <div className="flex flex-col gap-y-4">
            <p className="text-paragraph-sm text-foreground-muted">{t('connectionRequired')}</p>
            <Button asChild variant="outline">
              <Link href={ROUTES.arcaSettings}>{t('connection')}</Link>
            </Button>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
