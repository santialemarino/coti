'use client';

import { useId } from 'react';
import Link from 'next/link';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Badge,
  Button,
  Callout,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormRootMessage,
  Hint,
  InlineLink,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
} from '@repo/ui/components';
import { sendRecoveryCode, type RecoveryContact } from '@/app/forgot-password/actions';
import { ROUTES } from '@/config/routes';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface RecoveryContactStepProps {
  taxId: string;
  contacts: RecoveryContact[];
  onSent: (challengeId: string, contact: RecoveryContact) => void;
  onBack: () => void;
}

export function RecoveryContactStep({ taxId, contacts, onSent, onBack }: RecoveryContactStepProps) {
  const t = useTranslations('customerRecovery');
  const id = useId();
  const form = useForm({
    ...FORM_VALIDATION,
    defaultValues: { contactId: contacts.find((contact) => contact.preselected)?.id ?? '' },
  });
  const usable = contacts.some((contact) => contact.available);
  const pending = form.formState.isSubmitting;

  async function submit(values: { contactId: string }) {
    const contact = contacts.find(
      (candidate) => candidate.id === values.contactId && candidate.available,
    );
    if (!contact) return;
    const result = await sendRecoveryCode(taxId, contact.id);
    if (result.error || !result.id) {
      form.setError('root', { message: t(`errors.${result.error ?? 'unavailable'}`) });
      return;
    }
    onSent(result.id, contact);
  }

  if (contacts.length === 0)
    return (
      <div className="flex flex-col gap-y-5">
        <Callout tone="warning">{t('noContacts')}</Callout>
        <p className="text-paragraph-sm text-foreground-muted">
          {t('noAccount')}{' '}
          <InlineLink asChild>
            <Link href={ROUTES.signup}>{t('createAccount')}</Link>
          </InlineLink>
        </p>
        <Button variant="outline" onClick={onBack} autoFocus>
          {t('otherTaxId')}
        </Button>
      </div>
    );

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(submit)} noValidate className="flex flex-col gap-y-5">
        <p className="text-paragraph-sm text-foreground-muted">{t('choose')}</p>
        <FormField
          control={form.control}
          name="contactId"
          render={({ field }) => (
            <FormItem>
              <FormControl>
                <RadioGroup
                  aria-label={t('choose')}
                  className="gap-y-3"
                  value={field.value}
                  onValueChange={field.onChange}
                  disabled={pending}
                >
                  {contacts.map((contact) => (
                    <div key={contact.id} className="flex items-start gap-x-2.5">
                      <RadioGroupItem
                        id={`${id}-${contact.id}`}
                        value={contact.id}
                        disabled={!contact.available}
                        autoFocus={contact.id === field.value}
                        className="mt-0.5"
                      />
                      <div className="flex flex-col gap-y-0.5">
                        <Label
                          htmlFor={`${id}-${contact.id}`}
                          className="flex flex-wrap items-center gap-x-2 cursor-pointer"
                        >
                          {t(contact.kind === 'WHATSAPP' ? 'whatsapp' : 'email', {
                            contact: contact.masked,
                          })}
                          {contact.main ? <Badge size="sm">{t('main')}</Badge> : null}
                        </Label>
                        {contact.available ? null : <Hint>{t('whatsappSoon')}</Hint>}
                      </div>
                    </div>
                  ))}
                </RadioGroup>
              </FormControl>
            </FormItem>
          )}
        />
        {usable ? null : <Callout tone="warning">{t('whatsappOnly')}</Callout>}
        <FormRootMessage />
        {usable ? (
          <PendingButton type="submit" pending={pending} pendingLabel={t('sending')}>
            {t('send')}
          </PendingButton>
        ) : null}
        <InlineLink asChild tone="muted" className="self-center">
          <button type="button" onClick={onBack} autoFocus={!usable}>
            {t('otherTaxId')}
          </button>
        </InlineLink>
      </form>
    </Form>
  );
}
