'use client';

import { useEffect, useRef, useState } from 'react';
import Image from 'next/image';
import { zodResolver } from '@hookform/resolvers/zod';
import { Building2Icon, CheckIcon, FileTextIcon, UploadIcon } from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';
import { useForm, type Resolver } from 'react-hook-form';

import {
  Button,
  Dropzone,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
  PhoneInput,
  StatusScreen,
  TaxIdInput,
  Textarea,
  ToggleGroup,
  ToggleGroupItem,
} from '@repo/ui/components';
import { EASE, MOTION } from '@repo/ui/lib';
import {
  contactStepSchema,
  EMPTY_PUBLIC_RFQ,
  orderSchema,
  type PublicRfqValues,
} from '@/app/rfq/[channelId]/form-schema';
import { ROUTES } from '@/config/routes';
import type { PublicRfqDestination } from '@/lib/api/public-rfqs';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { useFormatters } from '@/lib/i18n/formatters';

interface PublicRfqFormProps {
  channelId: string;
  destination: PublicRfqDestination;
  taxId?: string;
}
type Step = 'contact' | 'order' | 'received';
const CONTACT_FIELDS = ['taxId', 'name', 'email', 'phone'] as const;
const FILE_ERRORS = new Set(['FILE_TOO_LARGE', 'UNSUPPORTED_FILE_TYPE', 'LEGACY_EXCEL_FILE']);

export function PublicRfqForm({ channelId, destination, taxId }: PublicRfqFormProps) {
  const t = useTranslations('rfq');
  const shared = useTranslations('common.form.errors');
  const fmt = useFormatters();
  const reduced = useReducedMotion();
  const [step, setStep] = useState<Step>('contact');
  const stepRef = useRef<Step>('contact');
  const attempted = useRef(false);
  const submitting = useRef(false);
  const validating = useRef(false);
  const heading = useRef<HTMLHeadingElement>(null);
  const receipt = useRef<HTMLDivElement>(null);
  const lastSubmission = useRef<{ key: string; file: File | undefined; requestId: string } | null>(
    null,
  );
  const text = {
    field: (key: string, values?: Record<string, string | number>) =>
      t(`validation.${key}`, values),
    shared,
  };
  const contact = contactStepSchema(text);
  const order = orderSchema(
    destination.maxTextCharacters,
    destination.maxFileSize,
    destination.acceptedContentTypes,
    text,
  );
  const resolver: Resolver<PublicRfqValues> = async (values, context, options) => {
    if (!attempted.current) return { values, errors: {} };
    const resolve = zodResolver(stepRef.current === 'contact' ? contact : order);
    const result = await resolve(values, context, options);
    return {
      ...result,
      values: Object.keys(result.errors).length ? {} : { ...values, ...result.values },
    };
  };
  const form = useForm<PublicRfqValues>({
    ...FORM_VALIDATION,
    defaultValues: { ...EMPTY_PUBLIC_RFQ, taxId: taxId ?? '' },
    resolver,
  });
  const mode = form.watch('mode');
  const file = form.watch('file');
  const pending = form.formState.isSubmitting;

  useEffect(() => {
    heading.current?.focus();
  }, [step]);

  function goTo(next: Step) {
    attempted.current = false;
    stepRef.current = next;
    form.clearErrors();
    setStep(next);
  }

  async function onValid(values: PublicRfqValues) {
    if (stepRef.current === 'contact') {
      goTo('order');
      return;
    }
    if (submitting.current) return;
    submitting.current = true;
    try {
      const snapshot = JSON.stringify({
        taxId: values.taxId,
        name: values.name,
        email: values.email,
        phone: values.phone,
        mode: values.mode,
        text: values.mode === 'text' ? values.text.trim() : '',
      });
      const selectedFile = values.mode === 'file' ? values.file : undefined;
      if (
        lastSubmission.current?.key !== snapshot ||
        lastSubmission.current?.file !== selectedFile
      ) {
        lastSubmission.current = {
          key: snapshot,
          file: selectedFile,
          requestId: crypto.randomUUID(),
        };
      }
      const body = new FormData();
      body.set('request_id', lastSubmission.current.requestId);
      body.set('tax_id', values.taxId);
      body.set('name', values.name);
      body.set('email', values.email);
      body.set('phone', values.phone);
      if (selectedFile) body.set('file', selectedFile);
      const payload = selectedFile
        ? body
        : JSON.stringify({ ...Object.fromEntries(body.entries()), text: values.text.trim() });
      const response = await fetch(ROUTES.rfqSubmit(channelId), {
        method: 'POST',
        body: payload,
        headers: selectedFile ? undefined : { 'Content-Type': 'application/json' },
      });
      const result = (await response.json()) as { received?: boolean; code?: string };
      if (!response.ok || result.received !== true) {
        const code = result.code && t.has(`errors.${result.code}`) ? result.code : 'INTERNAL';
        form.setError(values.mode === 'file' && FILE_ERRORS.has(code) ? 'file' : 'root', {
          message: t(`errors.${code}`),
        });
        return;
      }
      form.reset({ ...EMPTY_PUBLIC_RFQ, taxId: taxId ?? '' });
      lastSubmission.current = null;
      goTo('received');
    } catch {
      form.setError('root', { message: t('errors.UNREACHABLE') });
    } finally {
      submitting.current = false;
    }
  }

  return (
    <main className="flex flex-col min-h-dvh items-center px-4 py-8 gap-y-8 sm:px-6 sm:py-12">
      <header className="flex w-full max-w-xl items-center gap-x-3">
        {destination.logoUrl ? (
          <Image
            src={destination.logoUrl}
            alt=""
            width={48}
            height={48}
            unoptimized
            className="size-12 rounded-lg object-contain"
          />
        ) : (
          <span className="flex size-12 shrink-0 items-center justify-center bg-accent rounded-lg text-primary">
            <Building2Icon className="size-6" aria-hidden="true" />
          </span>
        )}
        <div className="min-w-0">
          <p className="break-words text-heading-5">{destination.supplierName}</p>
          <p className="text-paragraph-sm text-foreground-muted">{destination.branchName}</p>
        </div>
      </header>
      <section
        className="w-full max-w-xl p-6 border border-border bg-card rounded-1.5xl shadow-e2 sm:p-8"
        aria-label={t('title')}
      >
        <AnimatePresence initial={false} mode="wait">
          <motion.div
            key={step}
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onAnimationComplete={() => {
              if (stepRef.current === step) {
                if (step === 'received') receipt.current?.focus();
                else heading.current?.focus();
              }
            }}
            transition={{ duration: reduced ? 0 : MOTION.fast, ease: EASE.outSoft }}
          >
            {step === 'received' ? (
              <div ref={receipt} role="status" tabIndex={-1} className="outline-none">
                <StatusScreen
                  icon={CheckIcon}
                  tone="success"
                  title={t('received.title')}
                  description={t('received.description')}
                >
                  <Button onClick={() => goTo('contact')}>{t('received.another')}</Button>
                </StatusScreen>
              </div>
            ) : (
              <>
                <h1 ref={heading} tabIndex={-1} className="mb-6 outline-none text-heading-3">
                  {t(`${step}.title`)}
                </h1>
                <Form {...form}>
                  <form
                    noValidate
                    autoComplete="off"
                    className="flex flex-col gap-y-5"
                    onSubmit={(event) => {
                      if (submitting.current || validating.current || stepRef.current !== step) {
                        event.preventDefault();
                        return;
                      }
                      attempted.current = true;
                      validating.current = true;
                      void form
                        .handleSubmit(onValid, (errors) => {
                          if (CONTACT_FIELDS.some((field) => errors[field])) {
                            stepRef.current = 'contact';
                            setStep('contact');
                          }
                        })(event)
                        .finally(() => {
                          validating.current = false;
                        });
                    }}
                  >
                    {step === 'contact' ? (
                      <>
                        <FormField
                          control={form.control}
                          name="taxId"
                          render={({ field }) => (
                            <FormItem required>
                              <FormLabel>{t('fields.taxId.label')}</FormLabel>
                              <FormControl>
                                <TaxIdInput
                                  readOnly={Boolean(taxId)}
                                  {...field}
                                  kind="cuit"
                                  placeholder={t('fields.taxId.placeholder')}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        {(['name', 'email'] as const).map((name) => (
                          <FormField
                            key={name}
                            control={form.control}
                            name={name}
                            render={({ field }) => (
                              <FormItem required>
                                <FormLabel>{t(`fields.${name}.label`)}</FormLabel>
                                <FormControl>
                                  <Input
                                    {...field}
                                    type={name === 'email' ? 'email' : 'text'}
                                    autoComplete="off"
                                    maxLength={255}
                                    placeholder={t(`fields.${name}.placeholder`)}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        ))}
                        <FormField
                          control={form.control}
                          name="phone"
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('fields.phone.label')}</FormLabel>
                              <FormControl>
                                <PhoneInput
                                  value={field.value}
                                  onChange={field.onChange}
                                  onBlur={field.onBlur}
                                  name={field.name}
                                  autoComplete="off"
                                  formatters={fmt}
                                  placeholder={t('fields.phone.placeholder')}
                                  countryLabel={t('fields.phone.country')}
                                  countrySearchPlaceholder={t('fields.phone.search')}
                                  countryEmptyLabel={t('fields.phone.empty')}
                                  landlineNotice={{
                                    message: t('fields.phone.landline'),
                                    action: (number) => t('fields.phone.mobile', { number }),
                                  }}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </>
                    ) : (
                      <>
                        <ToggleGroup
                          type="single"
                          value={mode}
                          onValueChange={(value) => {
                            if (value === 'text' || value === 'file') {
                              form.setValue('mode', value);
                              form.clearErrors(['text', 'file']);
                              form.clearErrors('root');
                            }
                          }}
                          disabled={pending}
                          className="w-full"
                          aria-label={t('order.mode')}
                        >
                          <ToggleGroupItem
                            value="text"
                            className="h-auto min-h-8 min-w-0 whitespace-normal"
                          >
                            <FileTextIcon aria-hidden="true" className="hidden sm:block" />
                            {t('order.textMode')}
                          </ToggleGroupItem>
                          <ToggleGroupItem
                            value="file"
                            className="h-auto min-h-8 min-w-0 whitespace-normal"
                          >
                            <UploadIcon aria-hidden="true" className="hidden sm:block" />
                            {t('order.fileMode')}
                          </ToggleGroupItem>
                        </ToggleGroup>
                        {mode === 'text' ? (
                          <FormField
                            control={form.control}
                            name="text"
                            render={({ field }) => (
                              <FormItem required>
                                <FormLabel>{t('fields.text.label')}</FormLabel>
                                <FormControl>
                                  <Textarea
                                    {...field}
                                    disabled={pending}
                                    maxLength={destination.maxTextCharacters}
                                    rows={9}
                                    className="min-h-56"
                                    placeholder={t('fields.text.placeholder')}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        ) : (
                          <FormField
                            control={form.control}
                            name="file"
                            render={({ field }) => (
                              <FormItem required>
                                <FormLabel>{t('fields.file.label')}</FormLabel>
                                <FormControl>
                                  <Dropzone
                                    accept={destination.acceptedContentTypes.join(',')}
                                    onFile={(chosen) => field.onChange(chosen)}
                                    disabled={pending}
                                    title={t('fields.file.drop')}
                                    chooseLabel={
                                      file ? t('fields.file.replace') : t('fields.file.choose')
                                    }
                                    releaseLabel={t('fields.file.release')}
                                    fileName={file?.name}
                                    hint={t('fields.file.formats', {
                                      size: fmt.value(destination.maxFileSize / 1024 / 1024),
                                    })}
                                  />
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        )}
                      </>
                    )}
                    <FormRootMessage />
                    <div className="flex flex-col gap-y-3 sm:flex-row sm:items-center sm:justify-between sm:gap-x-3">
                      {step === 'order' ? (
                        <Button
                          variant="outline"
                          disabled={pending}
                          onClick={() => goTo('contact')}
                          className="sm:w-auto"
                        >
                          {t('order.editContact')}
                        </Button>
                      ) : null}
                      <PendingButton
                        type="submit"
                        pending={pending && step === 'order'}
                        disabled={pending}
                        pendingLabel={t('submitting')}
                        className="sm:ml-auto"
                      >
                        {t(step === 'contact' ? 'contact.continue' : 'submit')}
                      </PendingButton>
                    </div>
                  </form>
                </Form>
              </>
            )}
          </motion.div>
        </AnimatePresence>
      </section>
    </main>
  );
}
