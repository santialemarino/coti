'use client';

import { useId, useState } from 'react';
import { useRouter } from 'next/navigation';
import { CombineIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
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
  FormRootMessage,
  Hint,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
  Skeleton,
} from '@repo/ui/components';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { clientDisplayName, type Client } from '@/lib/api/client-profiles';
import { getMergeCandidates, mergeClient } from '@/lib/api/clients-client';
import { errorCodeOf } from '@/lib/api/errors';

type Survivor = 'this' | 'other';

interface MergeValues {
  otherId: string | null;
  survivor: Survivor;
}

interface ClientMergeDialogProps {
  clientId: string;
  name: string;
}

/*
 * Joins two provisional profiles of the same customer, such as a counter buyer who writes from two
 * numbers. Only profiles without a CUIT whose orders are all in the seller's branches are offered.
 */
export function ClientMergeDialog({ clientId, name }: ClientMergeDialogProps) {
  const router = useRouter();
  const t = useTranslations('clients.merge');
  const tClients = useTranslations('clients');
  const message = useApiErrorMessage('clients.merge');
  const fieldId = useId();
  const [open, setOpen] = useState(false);
  const [candidates, setCandidates] = useState<Client[] | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const form = useForm<MergeValues>({ defaultValues: { otherId: null, survivor: 'this' } });
  const otherId = useWatch({ control: form.control, name: 'otherId' });
  const survivor = useWatch({ control: form.control, name: 'survivor' });
  const pending = form.formState.isSubmitting;
  const other = candidates?.find((candidate) => candidate.id === otherId) ?? null;
  const otherName = other ? clientDisplayName(other, tClients('unnamed')) : '';

  async function load() {
    setCandidates(null);
    setLoadFailed(false);
    try {
      setCandidates(await getMergeCandidates(clientId));
    } catch {
      setLoadFailed(true);
    }
  }

  function onOpenChange(next: boolean) {
    if (pending) return;
    if (next) {
      form.reset({ otherId: null, survivor: 'this' });
      void load();
    }
    setOpen(next);
  }

  async function onConfirm() {
    if (!other) return;
    const [target, source] = survivor === 'this' ? [clientId, other.id] : [other.id, clientId];
    try {
      await mergeClient(target, source);
      const kept = survivor === 'this' ? name : otherName;
      toast.success(t('merged', { kept, merged: survivor === 'this' ? otherName : name }));
      setOpen(false);
      if (target === clientId) router.refresh();
      else router.push(ROUTES.clientDetail(target));
    } catch (cause) {
      form.setError('root', { message: message(errorCodeOf(cause)) });
    }
  }

  return (
    <>
      <Button variant="outline" onClick={() => onOpenChange(true)}>
        <CombineIcon aria-hidden="true" className="size-4" />
        {t('open')}
      </Button>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent closeOnClickOutside={otherId === null}>
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription>{t('description', { name })}</DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onConfirm)}
              noValidate
              aria-label={t('title')}
              className="flex flex-col gap-y-5"
            >
              <div className="flex flex-col gap-y-2">
                <Label htmlFor={`${fieldId}-other`}>{t('other.label')}</Label>
                {loadFailed ? (
                  <div className="flex flex-col items-start gap-y-2">
                    <p role="alert" className="text-paragraph-sm text-danger-foreground">
                      {t('loadError')}
                    </p>
                    <Button variant="outline" size="sm" onClick={() => void load()}>
                      {t('retry')}
                    </Button>
                  </div>
                ) : candidates === null ? (
                  <Skeleton className="w-full h-9" />
                ) : candidates.length === 0 ? (
                  <Hint>{t('other.none')}</Hint>
                ) : (
                  <Combobox
                    id={`${fieldId}-other`}
                    value={otherId}
                    onValueChange={(value) => form.setValue('otherId', value)}
                    disabled={pending}
                    searchable
                    placeholder={t('other.placeholder')}
                    searchPlaceholder={t('other.search')}
                    emptyLabel={t('other.empty')}
                    options={candidates.map((candidate) => ({
                      value: candidate.id,
                      label: [
                        clientDisplayName(candidate, tClients('unnamed')),
                        candidate.phone ?? candidate.email,
                      ]
                        .filter(Boolean)
                        .join(' · '),
                    }))}
                  />
                )}
              </div>
              {other ? (
                <div className="flex flex-col gap-y-2">
                  <Label id={`${fieldId}-survivor`}>{t('survivor.label')}</Label>
                  <RadioGroup
                    aria-labelledby={`${fieldId}-survivor`}
                    className="gap-y-3"
                    value={survivor}
                    onValueChange={(value) => form.setValue('survivor', value as Survivor)}
                    disabled={pending}
                  >
                    {(['this', 'other'] as const).map((option) => (
                      <div key={option} className="flex items-start gap-x-2.5">
                        <RadioGroupItem
                          id={`${fieldId}-survivor-${option}`}
                          value={option}
                          className="mt-0.5"
                        />
                        <Label htmlFor={`${fieldId}-survivor-${option}`} className="cursor-pointer">
                          {option === 'this' ? name : otherName}
                        </Label>
                      </div>
                    ))}
                  </RadioGroup>
                  <Hint>
                    {t('survivor.hint', { merged: survivor === 'this' ? otherName : name })}
                  </Hint>
                </div>
              ) : null}
              <FormRootMessage />
              <DialogFooter>
                <Button variant="outline" disabled={pending} onClick={() => onOpenChange(false)}>
                  {t('cancel')}
                </Button>
                <PendingButton
                  type="submit"
                  pending={pending}
                  pendingLabel={t('submitting')}
                  disabled={!other}
                >
                  {t('submit')}
                </PendingButton>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </>
  );
}
