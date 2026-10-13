'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { CheckCircle2Icon, UploadIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
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
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { cn } from '@repo/ui/lib';
import { uploadARCACertificate } from '@/app/(protected)/settings/invoicing/arca/actions';
import {
  arcaCertificateSchema,
  certificateContent,
  type ARCACertificateValues,
} from '@/app/(protected)/settings/invoicing/arca/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { CERTIFICATE_MAX_BYTES, CERTIFICATE_MAX_KB } from '@/lib/constants/arca';
import { FORM_VALIDATION } from '@/lib/forms/options';

// What a saved certificate, or a mistaken file, is usually called. The content decides, not this.
const CERTIFICATE_FILE_TYPES = '.crt,.pem,.cer,.csr,.txt';

interface CertificateUploadProps {
  disabled?: boolean;
  // ARCA hands the certificate over as a file, so choosing it leads and pasting is the fallback.
  filePrimary?: boolean;
}

/*
 * The certificate ARCA issued, pasted as text. A file, chosen or dropped onto the field, only fills
 * the same field, so there is one thing to check and one way to save it.
 */
export function CertificateUpload({
  disabled = false,
  filePrimary = false,
}: CertificateUploadProps) {
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
  // A refusal arrives while the field is still disabled for the save, so focus waits for it to open.
  const focusAfterSave = useRef(false);
  const loadedName = useHeldWhileClosed(loadedFrom, loadedFrom !== null);

  useEffect(() => {
    if (pending || !focusAfterSave.current) return;
    focusAfterSave.current = false;
    form.setFocus('certificate');
  }, [pending, form]);

  async function readFile(file: File | undefined) {
    if (!file) return;
    if (file.size > CERTIFICATE_MAX_BYTES) {
      setLoadedFrom(null);
      form.setError('certificate', {
        message: t('steps.upload.tooLarge', { max: CERTIFICATE_MAX_KB }),
      });
      return;
    }
    let text: string;
    try {
      text = await file.text();
    } catch {
      form.setError('certificate', { message: t('steps.upload.readError') });
      return;
    }
    const content = certificateContent(text);
    if (content !== 'certificate') {
      setLoadedFrom(null);
      form.setError(
        'certificate',
        {
          message: t(
            content === 'request' ? 'steps.upload.fileRequest' : 'steps.upload.fileInvalid',
          ),
        },
        { shouldFocus: true },
      );
      return;
    }
    form.setValue('certificate', text, { shouldValidate: true, shouldDirty: true });
    setLoadedFrom(file.name);
  }

  function carriesFiles(event: React.DragEvent) {
    return Array.from(event.dataTransfer.types).includes('Files');
  }

  async function onSubmit(values: ARCACertificateValues) {
    const result = await uploadARCACertificate(values.certificate);
    if (result.error) {
      form.setError('certificate', { message: message(result.error) });
      focusAfterSave.current = true;
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
              {filePrimary ? (
                <Button
                  variant="outline"
                  className="self-start"
                  disabled={locked}
                  onClick={() => fileInput.current?.click()}
                >
                  <UploadIcon aria-hidden="true" />
                  {t('steps.upload.chooseCertificate')}
                </Button>
              ) : null}
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
                  placeholder={t(
                    filePrimary ? 'steps.upload.pastePlaceholder' : 'steps.upload.placeholder',
                  )}
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
              {filePrimary ? null : (
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
              )}
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
              <p data-slot="certificate-loaded" aria-live="polite" className="sr-only">
                {loadedFrom ? t('steps.upload.loaded', { fileName: loadedFrom }) : null}
              </p>
              {/* The visible note reveals and collapses like a field message, holding its text while
                  it leaves; the live region above is what gets announced. */}
              <div
                aria-hidden="true"
                data-shown={loadedFrom !== null}
                className="grid grid-rows-[0fr] -mt-2 transition-[grid-template-rows] duration-200 ease-out-soft motion-reduce:transition-none data-[shown=true]:grid-rows-[1fr] [&>*]:overflow-hidden"
              >
                <div>
                  <p
                    className={cn(
                      'flex items-start pt-2 gap-x-1.5 text-paragraph-xs text-success-foreground transition-opacity duration-200 ease-out-soft motion-reduce:transition-none',
                      loadedFrom === null && 'opacity-0',
                    )}
                  >
                    <CheckCircle2Icon aria-hidden="true" className="size-3.5 mt-0.5 shrink-0" />
                    {loadedName ? t('steps.upload.loaded', { fileName: loadedName }) : null}
                  </p>
                </div>
              </div>
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
