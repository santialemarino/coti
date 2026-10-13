'use client';

import { useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  InlineLink,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
import { cn } from '@repo/ui/lib';
import { uploadARCACertificate } from '@/app/(protected)/settings/invoicing/arca/actions';
import {
  arcaCertificateSchema,
  type ARCACertificateValues,
} from '@/app/(protected)/settings/invoicing/arca/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { CERTIFICATE_MAX_BYTES, CERTIFICATE_MAX_KB } from '@/lib/constants/arca';
import { FORM_VALIDATION } from '@/lib/forms/options';

const CERTIFICATE_FILE_TYPES = '.crt,.pem,.cer';

interface CertificateUploadProps {
  disabled?: boolean;
}

/*
 * The certificate ARCA issued, pasted as text. A file, chosen or dropped onto the field, only fills
 * the same field, so there is one thing to check and one way to save it.
 */
export function CertificateUpload({ disabled = false }: CertificateUploadProps) {
  const router = useRouter();
  const t = useTranslations('arca');
  const tShared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('arca');
  const fileInput = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [loadedFrom, setLoadedFrom] = useState<string | null>(null);
  const schema = useMemo(() => arcaCertificateSchema({ field: t, shared: tShared }), [t, tShared]);
  const form = useForm<ARCACertificateValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { certificate: '' },
  });
  const pending = form.formState.isSubmitting;
  const locked = disabled || pending;

  async function readFile(file: File | undefined) {
    if (!file) return;
    if (file.size > CERTIFICATE_MAX_BYTES) {
      form.setError('certificate', {
        message: t('steps.upload.tooLarge', { max: CERTIFICATE_MAX_KB }),
      });
      return;
    }
    try {
      form.setValue('certificate', await file.text(), { shouldValidate: true, shouldDirty: true });
      setLoadedFrom(file.name);
    } catch {
      form.setError('certificate', { message: t('steps.upload.readError') });
    }
  }

  function carriesFiles(event: React.DragEvent) {
    return Array.from(event.dataTransfer.types).includes('Files');
  }

  async function onSubmit(values: ARCACertificateValues) {
    const result = await uploadARCACertificate(values.certificate);
    if (result.error) {
      form.setError('certificate', { message: message(result.error) }, { shouldFocus: true });
      return;
    }
    toast.success(t('steps.upload.saved'));
    router.refresh();
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="flex flex-col gap-y-4">
        <FormField
          control={form.control}
          name="certificate"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>{t('steps.upload.label')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  onChange={(event) => {
                    field.onChange(event);
                    setLoadedFrom(null);
                  }}
                  rows={6}
                  spellCheck={false}
                  autoComplete="off"
                  disabled={locked}
                  placeholder={t('steps.upload.placeholder')}
                  data-dragging={dragging || undefined}
                  onDragOver={(event) => {
                    if (!carriesFiles(event)) return;
                    event.preventDefault();
                    setDragging(true);
                  }}
                  onDragLeave={() => setDragging(false)}
                  onDrop={(event) => {
                    setDragging(false);
                    if (!carriesFiles(event)) return;
                    event.preventDefault();
                    void readFile(event.dataTransfer.files[0]);
                  }}
                  className={cn(
                    'max-h-64 transition-[border-color,box-shadow,background-color] text-paragraph-xs font-mono',
                    dragging && 'bg-accent border-ring ring-3 ring-ring/45',
                  )}
                />
              </FormControl>
              <p className="text-paragraph-xs text-foreground-muted">
                {t('steps.upload.fromFile')}{' '}
                <InlineLink asChild>
                  <button
                    type="button"
                    disabled={locked}
                    onClick={() => fileInput.current?.click()}
                  >
                    {t('steps.upload.chooseFile')}
                  </button>
                </InlineLink>
              </p>
              <input
                ref={fileInput}
                type="file"
                accept={CERTIFICATE_FILE_TYPES}
                className="hidden"
                tabIndex={-1}
                onChange={(event) => {
                  void readFile(event.target.files?.[0]);
                  event.target.value = '';
                }}
              />
              <p
                aria-live="polite"
                className="text-paragraph-xs text-foreground-muted empty:hidden"
              >
                {loadedFrom ? t('steps.upload.loaded', { fileName: loadedFrom }) : null}
              </p>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormRootMessage />
        <PendingButton
          type="submit"
          className="self-start"
          pending={pending}
          disabled={disabled}
          pendingLabel={t('steps.upload.submitting')}
        >
          {t('steps.upload.submit')}
        </PendingButton>
      </form>
    </Form>
  );
}
