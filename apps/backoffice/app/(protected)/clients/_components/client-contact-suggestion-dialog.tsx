'use client';

import { useId, useState } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Callout,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormRootMessage,
  Hint,
  InlineLink,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
} from '@repo/ui/components';
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import {
  isMergeable,
  joinableAccountId,
  resolveContactSuggestion,
  type ContactSuggestion,
} from '@/lib/api/client-contact-suggestions';
import { errorCodeOf } from '@/lib/api/errors';

interface ClientContactSuggestionDialogProps {
  suggestion: ContactSuggestion | null;
  branchId: string | null;
  onClose: () => void;
  onResolved: () => void;
}

interface DecisionValues {
  keepClientId: string;
}

/*
 * Two profiles found sharing one contact. Between two provisional profiles the seller merges them or
 * dismisses the case. With a customer account on one side the seller joins the other profile into it
 * once the customer proved the contact from the portal; until then dismissing is all.
 */
export function ClientContactSuggestionDialog({
  suggestion,
  branchId,
  onClose,
  onResolved,
}: ClientContactSuggestionDialogProps) {
  const t = useTranslations('clientContactSuggestions');
  const message = useApiErrorMessage('clientContactSuggestions');
  const fieldId = useId();
  const open = suggestion !== null;
  const shown = useHeldWhileClosed(suggestion, open);
  const form = useForm<DecisionValues>({ defaultValues: { keepClientId: '' } });
  const keepClientId = useWatch({ control: form.control, name: 'keepClientId' });
  const [pending, setPending] = useState<'MERGE' | 'SEPARATE' | null>(null);
  const [opened, setOpened] = useState<string | null>(null);

  if (suggestion && opened !== suggestion.id) {
    setOpened(suggestion.id);
    form.reset({ keepClientId: suggestion.candidateClientId ?? '' });
  }

  async function decide(action: 'MERGE' | 'SEPARATE') {
    if (!shown || pending) return;
    setPending(action);
    form.clearErrors('root');
    // The customer account always survives a join, whatever the radio last held.
    const account = joinableAccountId(shown);
    const keep = account ?? keepClientId;
    try {
      await resolveContactSuggestion(
        shown.id,
        action === 'MERGE' ? { action, keepClientId: keep } : { action },
        branchId,
      );
      const kept = keep === shown.sourceClientId ? shown.sourceName : shown.candidateName;
      toast.success(
        action === 'MERGE'
          ? t(account ? 'joined' : 'merged', { name: kept ?? t('unknown') })
          : t('separated', { value: shown.value }),
      );
      onResolved();
    } catch (cause) {
      form.setError('root', { message: message(errorCodeOf(cause)) });
    } finally {
      setPending(null);
    }
  }

  const mergeable = shown ? isMergeable(shown) : false;
  const joinable = shown ? joinableAccountId(shown) !== null : false;
  const profiles = shown
    ? [
        { id: shown.candidateClientId, name: shown.candidateName, label: t('candidate') },
        { id: shown.sourceClientId, name: shown.sourceName, label: t('source') },
      ]
    : [];

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !pending) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('reviewTitle')}</DialogTitle>
          <DialogDescription>
            {t('reviewDescription', { value: shown?.value ?? '' })}
          </DialogDescription>
        </DialogHeader>
        {shown ? (
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(() => decide('MERGE'))}
              noValidate
              aria-label={t('reviewTitle')}
              className="flex flex-col gap-y-4"
            >
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-paragraph-sm">
                <dt>{t('incoming')}</dt>
                <dd className="wrap-anywhere">{shown.value}</dd>
                {shown.taxId ? (
                  <>
                    <dt>{t('taxId')}</dt>
                    <dd>{shown.taxId}</dd>
                  </>
                ) : null}
                {profiles.map((profile) => (
                  <ProfileRow key={profile.label} {...profile} fallback={t('unknown')} />
                ))}
              </dl>
              {mergeable ? (
                <div className="flex flex-col gap-y-2">
                  <Label id={`${fieldId}-keep`}>{t('keep')}</Label>
                  <RadioGroup
                    aria-labelledby={`${fieldId}-keep`}
                    className="gap-y-3"
                    value={keepClientId}
                    onValueChange={(value) => form.setValue('keepClientId', value)}
                    disabled={pending !== null}
                  >
                    {profiles.map((profile) => (
                      <div key={profile.id} className="flex items-start gap-x-2.5">
                        <RadioGroupItem
                          id={`${fieldId}-keep-${profile.id}`}
                          value={profile.id ?? ''}
                          className="mt-0.5"
                        />
                        <Label htmlFor={`${fieldId}-keep-${profile.id}`} className="cursor-pointer">
                          {profile.name ?? t('unknown')}
                        </Label>
                      </div>
                    ))}
                  </RadioGroup>
                  <Hint>{t('keepHint')}</Hint>
                </div>
              ) : joinable ? (
                <Hint>{t('joinHint')}</Hint>
              ) : (
                <Callout>{t('registered')}</Callout>
              )}
              <FormRootMessage />
              <DialogFooter>
                <PendingButton
                  variant="outline"
                  pending={pending === 'SEPARATE'}
                  pendingLabel={t('separating')}
                  disabled={pending === 'MERGE'}
                  onClick={() => void decide('SEPARATE')}
                >
                  {t('separate')}
                </PendingButton>
                {mergeable || joinable ? (
                  <PendingButton
                    type="submit"
                    pending={pending === 'MERGE'}
                    pendingLabel={t('merging')}
                    disabled={pending === 'SEPARATE'}
                  >
                    {t(joinable ? 'join' : 'merge')}
                  </PendingButton>
                ) : null}
              </DialogFooter>
            </form>
          </Form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function ProfileRow({
  id,
  name,
  label,
  fallback,
}: {
  id: string | null;
  name: string | null;
  label: string;
  fallback: string;
}) {
  return (
    <>
      <dt>{label}</dt>
      <dd className="wrap-anywhere">
        {id ? (
          <InlineLink asChild>
            <Link href={ROUTES.clientDetail(id)}>{name ?? fallback}</Link>
          </InlineLink>
        ) : (
          fallback
        )}
      </dd>
    </>
  );
}
