'use client';

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';

import {
  Callout,
  ConfirmDialog,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  InlineLink,
  Input,
  PendingButton,
  Spinner,
} from '@repo/ui/components';
import { checkPortal, loadPortal, savePortal } from '@/components/customer-portal/actions';
import { portalPreview, portalSchema } from '@/components/customer-portal/form-schema';
import type { CustomerPortal } from '@/lib/api/customer-portal';
import { PORTAL_AVAILABILITY_DELAY_SECONDS } from '@/lib/config';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface CustomerPortalEditorProps {
  formId?: string;
  onReadyChange?: (ready: boolean) => void;
  onBusyChange?: (busy: boolean) => void;
  onSaved?: () => void | Promise<void>;
}

export function CustomerPortalEditor({
  formId,
  onSaved,
  onReadyChange,
  onBusyChange,
}: CustomerPortalEditorProps) {
  const router = useRouter();
  const savingRef = useRef(false);
  const t = useTranslations('customerPortal');
  const form = useForm({
    ...FORM_VALIDATION,
    resolver: zodResolver(portalSchema(t('invalid'))),
    defaultValues: { subdomain: '' },
  });
  const [portal, setPortal] = useState<CustomerPortal | null>(null);
  const [loadingError, setLoadingError] = useState(false);
  const [availability, setAvailability] = useState<
    'idle' | 'checking' | 'available' | 'taken' | 'invalid' | 'error'
  >('idle');
  const [confirmation, setConfirmation] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const subdomain = useWatch({ control: form.control, name: 'subdomain' });
  const normalized = subdomain.trim().toLowerCase();
  const disabled = !portal?.baseUrl || portal.legacy || saving;

  useEffect(() => {
    let active = true;
    void loadPortal().then((result) => {
      if (!active) return;
      if (result.data) {
        setPortal(result.data);
        form.reset({ subdomain: result.data.subdomain });
      } else setLoadingError(true);
    });
    return () => {
      active = false;
    };
  }, [form]);

  useEffect(() => {
    if (!portal?.baseUrl || portal.legacy || !normalized) {
      setAvailability('idle');
      return;
    }
    if (!portalSchema().safeParse({ subdomain }).success) {
      setAvailability('invalid');
      return;
    }
    let active = true;
    setAvailability('checking');
    const timer = setTimeout(() => {
      void checkPortal(subdomain).then((result) => {
        if (active)
          setAvailability(
            result.error === 'INVALID_INPUT'
              ? 'invalid'
              : result.error
                ? 'error'
                : result.available
                  ? 'available'
                  : 'taken',
          );
      });
    }, PORTAL_AVAILABILITY_DELAY_SECONDS * 1000);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [subdomain, normalized, portal]);

  useEffect(() => {
    onReadyChange?.(
      !disabled &&
        availability !== 'checking' &&
        availability !== 'taken' &&
        availability !== 'invalid',
    );
  }, [disabled, availability, onReadyChange]);
  useEffect(() => {
    onBusyChange?.(saving);
  }, [saving, onBusyChange]);

  async function save(value: string, confirmed: boolean) {
    if (!portal || savingRef.current) return;
    if (portal.hostname && value !== portal.subdomain && !confirmed) {
      setConfirmation(value);
      return;
    }
    savingRef.current = true;
    setSaving(true);
    try {
      const result = await savePortal(value, portal.hostname, confirmed);
      if (!result.ok) {
        form.setError(
          'subdomain',
          {
            message: t(
              result.error === 'CONFLICT'
                ? 'conflict'
                : result.error === 'INVALID_INPUT'
                  ? 'invalid'
                  : 'error',
            ),
          },
          { shouldFocus: true },
        );
        return;
      }
      const refreshed = await loadPortal();
      if (refreshed.data) {
        setPortal(refreshed.data);
        form.reset({ subdomain: refreshed.data.subdomain });
      }
      router.refresh();
      await onSaved?.();
    } finally {
      savingRef.current = false;
      setSaving(false);
      setConfirmation(null);
    }
  }

  if (!portal)
    return loadingError ? (
      <Callout tone="danger">{t('error')}</Callout>
    ) : (
      <div
        role="status"
        className="flex items-center gap-x-2 text-paragraph-sm text-foreground-muted"
      >
        <Spinner />
        {t('loading')}
      </div>
    );

  const fields = (
    <>
      {portal.url ? (
        <p className="break-all text-paragraph-sm text-foreground-muted">
          {t('current')}{' '}
          <InlineLink asChild>
            <a href={portal.url} target="_blank" rel="noreferrer">
              {portal.url}
            </a>
          </InlineLink>
        </p>
      ) : null}
      {portal.legacy ? (
        <Callout>{t('legacy')}</Callout>
      ) : !portal.baseUrl ? (
        <Callout>{t('unavailable')}</Callout>
      ) : null}
      {!portal.legacy && portal.baseUrl ? (
        <FormField
          control={form.control}
          name="subdomain"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('label')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  disabled={disabled}
                  onKeyDown={(event) => {
                    if (!formId && event.key === 'Enter') {
                      event.preventDefault();
                      event.stopPropagation();
                      void form.handleSubmit(({ subdomain: value }) => save(value, false))();
                    }
                  }}
                />
              </FormControl>
              <FormDescription>{t('hint')}</FormDescription>
              <p className="break-all text-paragraph-sm-medium text-foreground" aria-live="polite">
                {portalPreview(portal.baseUrl, subdomain)}
              </p>
              <FormDescription>
                <span role="status">{availability === 'idle' ? '' : t(availability)}</span>
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : null}
      {!formId && !portal.legacy && portal.baseUrl ? (
        <PendingButton
          pending={saving}
          pendingLabel={t('saving')}
          disabled={
            disabled ||
            availability === 'checking' ||
            availability === 'taken' ||
            availability === 'invalid'
          }
          onClick={form.handleSubmit(({ subdomain: value }) => save(value, false))}
        >
          {t('save')}
        </PendingButton>
      ) : null}
      {formId && (portal.legacy || !portal.baseUrl) ? (
        <p className="text-paragraph-sm text-foreground-muted">{t('skipHint')}</p>
      ) : null}
    </>
  );

  return (
    <Form {...form}>
      {formId ? (
        <form
          id={formId}
          noValidate
          className="flex max-w-md flex-col gap-y-4"
          onSubmit={form.handleSubmit(({ subdomain: value }) =>
            disabled ? undefined : save(value, false),
          )}
        >
          {fields}
        </form>
      ) : (
        <div className="flex flex-col gap-y-4">{fields}</div>
      )}
      <ConfirmDialog
        open={confirmation !== null}
        onOpenChange={(open) => !saving && !open && setConfirmation(null)}
        entity={portal.hostname}
        title={t('changeTitle')}
        description={() => t('changeWarning')}
        onConfirm={() => (confirmation ? save(confirmation, true) : undefined)}
        pending={saving}
        labels={{ confirm: t('confirm'), pending: t('saving'), cancel: t('cancel') }}
      />
    </Form>
  );
}
