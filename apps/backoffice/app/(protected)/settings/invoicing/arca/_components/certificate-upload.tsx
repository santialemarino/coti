'use client';

import { useMemo } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Dropzone,
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  PendingButton,
  Textarea,
} from '@repo/ui/components';
import { pemExample } from '@/app/(protected)/settings/invoicing/arca/_components/pem-example';
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

// The certificate ARCA issued, pasted as text or read from a file into the same field.
export function CertificateUpload({ disabled = false }: CertificateUploadProps) {
  const router = useRouter();
  const t = useTranslations('arca');
  const tShared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('arca');
  const schema = useMemo(() => arcaCertificateSchema({ field: t, shared: tShared }), [t, tShared]);
  const form = useForm<ARCACertificateValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: { certificate: '' },
  });
  const pending = form.formState.isSubmitting;

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
    } catch {
      form.setError('certificate', { message: t('steps.upload.readError') });
    }
  }

  async function onSubmit(values: ARCACertificateValues) {
    const result = await uploadARCACertificate(values.certificate);
    if (result.error) {
      form.setError('certificate', { message: message(result.error) });
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
                  rows={6}
                  spellCheck={false}
                  autoComplete="off"
                  disabled={disabled || pending}
                  placeholder={pemExample('CERTIFICATE')}
                  className="max-h-64 font-mono text-paragraph-xs"
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Dropzone
          accept={CERTIFICATE_FILE_TYPES}
          title={t('steps.upload.drop')}
          chooseLabel={t('steps.upload.choose')}
          releaseLabel={t('steps.upload.release')}
          hint={t('steps.upload.hint', { max: CERTIFICATE_MAX_KB })}
          disabled={disabled || pending}
          onFile={readFile}
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
