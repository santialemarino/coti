'use client';

import { useMemo, useState } from 'react';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';

import {
  Button,
  Card,
  CardContent,
  ConfirmDialog,
  Dropzone,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormRootMessage,
  Input,
  PendingButton,
  Separator,
} from '@repo/ui/components';
import {
  createARCASetup,
  disconnectARCA,
  uploadARCACertificate,
  verifyARCA,
} from '@/app/(protected)/settings/arca/actions';
import {
  arcaCertificateSchema,
  arcaIdentitySchema,
  arcaPointOfSaleSchema,
  type ARCACertificateValues,
  type ARCAIdentityValues,
  type ARCAPointOfSaleValues,
} from '@/app/(protected)/settings/arca/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ARCASetup } from '@/lib/api/arca-setup';
import { FORM_VALIDATION } from '@/lib/forms/options';
import { useFormatters } from '@/lib/i18n/formatters';

interface ARCASetupFormProps {
  setup: ARCASetup;
  branchId?: string;
}

export function ARCASetupForm({ setup, branchId }: ARCASetupFormProps) {
  const t = useTranslations('arca');
  const shared = useTranslations('common.form.errors');
  const message = useApiErrorMessage('arca');
  const fmt = useFormatters();
  const router = useRouter();
  const identitySchema = useMemo(() => arcaIdentitySchema({ field: t, shared }), [t, shared]);
  const pointSchema = useMemo(() => arcaPointOfSaleSchema({ field: t, shared }), [t, shared]);
  const certificateSchema = useMemo(() => arcaCertificateSchema({ field: t, shared }), [t, shared]);
  const identity = useForm<ARCAIdentityValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(identitySchema),
    defaultValues: { taxId: setup.taxId },
  });
  const point = useForm<ARCAPointOfSaleValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(pointSchema),
    defaultValues: { pointOfSale: setup.pointOfSale ? String(setup.pointOfSale) : '' },
  });
  const certificateForm = useForm<ARCACertificateValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(certificateSchema),
    defaultValues: { certificate: '' },
  });
  const [file, setFile] = useState<File>();
  const [uploading, setUploading] = useState(false);
  const [disconnectOpen, setDisconnectOpen] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);
  const [lastNumber, setLastNumber] = useState<number>();
  const busy =
    identity.formState.isSubmitting || point.formState.isSubmitting || uploading || disconnecting;

  async function create(values: ARCAIdentityValues) {
    const result = await createARCASetup(values);
    if (result.error) {
      identity.setError('root', { message: message(result.error) });
      return;
    }
    toast.success(t('requestCreated'));
    router.refresh();
  }

  function download() {
    const url = URL.createObjectURL(new Blob([setup.csr], { type: 'application/pkcs10' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = `coti-${setup.taxId}.csr`;
    link.click();
    URL.revokeObjectURL(url);
  }

  async function upload() {
    if (uploading) return;
    certificateForm.clearErrors();
    if (!file) {
      certificateForm.setError('certificate', { message: t('certificate.required') });
      return;
    }
    if (file.size > 16384) {
      certificateForm.setError('certificate', { message: t('certificate.tooLarge') });
      return;
    }
    setUploading(true);
    try {
      const result = await uploadARCACertificate(await file.text());
      if (result.error) {
        certificateForm.setError('root', { message: message(result.error) });
        return;
      }
      setFile(undefined);
      toast.success(t('certificate.saved'));
      router.refresh();
    } catch {
      certificateForm.setError('certificate', { message: t('certificate.readError') });
    } finally {
      setUploading(false);
    }
  }

  async function verify(values: ARCAPointOfSaleValues) {
    if (!branchId) {
      point.setError('root', { message: t('noBranch') });
      return;
    }
    setLastNumber(undefined);
    const result = await verifyARCA(values, branchId);
    if (result.error) {
      point.setError('root', { message: message(result.error) });
      return;
    }
    if (!result.connection?.verified) {
      point.setError('root', {
        message: t(`failures.${result.connection?.failure || 'UNAVAILABLE'}`),
      });
      return;
    }
    setLastNumber(result.connection.lastNumber);
    toast.success(t('verified'));
    router.refresh();
  }

  async function disconnect() {
    setDisconnecting(true);
    try {
      const result = await disconnectARCA();
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      setDisconnectOpen(false);
      setFile(undefined);
      setLastNumber(undefined);
      toast.success(t('disconnected'));
      router.refresh();
    } finally {
      setDisconnecting(false);
    }
  }

  return (
    <div className="flex flex-col gap-y-8">
      <section className="flex flex-col gap-y-4">
        <h2 className="text-heading-6">{t('tutorial.title')}</h2>
        <ol className="flex flex-col gap-y-3">
          {[
            'access',
            'generate',
            'downloadRequest',
            'certificate',
            'authorize',
            'upload',
            'verify',
          ].map((step, index) => (
            <li key={step}>
              <Card className="py-5">
                <CardContent className="flex items-start gap-x-4">
                  <span
                    aria-hidden="true"
                    className="flex size-8 shrink-0 items-center justify-center bg-primary/10 rounded-full text-paragraph-sm-semibold text-primary"
                  >
                    {index + 1}
                  </span>
                  <div className="flex flex-col min-w-0 gap-y-1.5">
                    <h3 className="text-paragraph-md-semibold">{t(`tutorial.headings.${step}`)}</h3>
                    <p className="text-paragraph-sm text-foreground-muted">
                      {t(`tutorial.${step}`)}
                    </p>
                  </div>
                </CardContent>
              </Card>
            </li>
          ))}
        </ol>
        <div className="flex flex-wrap gap-3">
          <Button asChild variant="outline">
            <a
              href="https://www.arca.gob.ar/fe/ayuda/entorno-prueba.asp"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('tutorial.official')}
            </a>
          </Button>
          <Button asChild variant="outline">
            <a
              href="https://www.arca.gob.ar/ws/WSASS/html/crearautorizacion.html"
              target="_blank"
              rel="noopener noreferrer"
            >
              {t('tutorial.authorization')}
            </a>
          </Button>
        </div>
      </section>
      <Separator />
      {!setup.enabled ? (
        <p role="status" className="text-paragraph-md text-muted-foreground">
          {t('disabled')}
        </p>
      ) : (
        <>
          <section className="flex flex-col max-w-xl gap-y-4">
            <h2 className="text-heading-6">{t('identity.title')}</h2>
            {!setup.csr ? (
              <Form {...identity}>
                <form
                  noValidate
                  onSubmit={identity.handleSubmit(create)}
                  className="flex flex-col gap-y-4"
                >
                  <FormField
                    control={identity.control}
                    name="taxId"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel required>{t('taxId.label')}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            maxLength={13}
                            placeholder={t('taxId.placeholder')}
                            disabled={busy}
                          />
                        </FormControl>
                        <FormDescription>{t('taxId.hint')}</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormRootMessage />
                  <PendingButton
                    type="submit"
                    pending={identity.formState.isSubmitting}
                    disabled={busy}
                    pendingLabel={t('creating')}
                  >
                    {t('create')}
                  </PendingButton>
                </form>
              </Form>
            ) : (
              <>
                <p className="text-paragraph-md">{t('identity.created', { cuit: setup.taxId })}</p>
                <Button variant="outline" onClick={download} disabled={busy}>
                  {t('download')}
                </Button>
              </>
            )}
          </section>
          {setup.csr && (
            <section className="flex flex-col max-w-xl gap-y-4">
              <h2 className="text-heading-6">{t('certificate.title')}</h2>
              {setup.hasCertificate ? (
                <p className="text-paragraph-md">
                  {t('certificate.installed', {
                    date: setup.certificateExpiresAt ? fmt.date(setup.certificateExpiresAt) : '',
                  })}
                </p>
              ) : (
                <Form {...certificateForm}>
                  <form
                    noValidate
                    onSubmit={certificateForm.handleSubmit(upload)}
                    className="flex flex-col gap-y-4"
                  >
                    <FormField
                      control={certificateForm.control}
                      name="certificate"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel required>{t('certificate.title')}</FormLabel>
                          <Dropzone
                            accept=".crt,.pem,.cer"
                            title={t('certificate.drop')}
                            chooseLabel={t('certificate.choose')}
                            releaseLabel={t('certificate.release')}
                            hint={t('certificate.hint')}
                            fileName={file?.name}
                            disabled={busy}
                            onFile={(value) => {
                              setFile(value);
                              field.onChange(value?.name ?? '');
                              certificateForm.clearErrors();
                            }}
                          />
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormRootMessage />
                    <PendingButton
                      type="submit"
                      pending={uploading}
                      disabled={busy || !file}
                      pendingLabel={t('certificate.uploading')}
                    >
                      {t('certificate.submit')}
                    </PendingButton>
                  </form>
                </Form>
              )}
            </section>
          )}
          {setup.hasCertificate && (
            <section className="flex flex-col max-w-xl gap-y-4">
              <h2 className="text-heading-6">{t('pointOfSale.title')}</h2>
              <p className="text-paragraph-sm text-muted-foreground">{t('pointOfSale.scope')}</p>
              {setup.verifiedAt && (
                <p className="text-paragraph-sm text-muted-foreground">
                  {t('lastVerified', {
                    date: fmt.timestamp(setup.verifiedAt),
                    point: setup.pointOfSale,
                  })}
                </p>
              )}
              {!branchId ? (
                <p className="text-paragraph-md">{t('noBranch')}</p>
              ) : (
                <Form {...point}>
                  <form
                    noValidate
                    onSubmit={point.handleSubmit(verify)}
                    className="flex flex-col gap-y-4"
                  >
                    <FormField
                      control={point.control}
                      name="pointOfSale"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel required>{t('pointOfSale.label')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              inputMode="numeric"
                              maxLength={5}
                              placeholder={t('pointOfSale.placeholder')}
                              disabled={busy}
                            />
                          </FormControl>
                          <FormDescription>{t('pointOfSale.hint')}</FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormRootMessage />
                    <PendingButton
                      type="submit"
                      pending={point.formState.isSubmitting}
                      disabled={busy}
                      pendingLabel={t('verifying')}
                    >
                      {t('verify')}
                    </PendingButton>
                    {lastNumber !== undefined && (
                      <p role="status" className="text-paragraph-sm">
                        {t('lastNumber', { number: lastNumber })}
                      </p>
                    )}
                  </form>
                </Form>
              )}
            </section>
          )}
          {setup.csr && (
            <div>
              <Button variant="outline" onClick={() => setDisconnectOpen(true)} disabled={busy}>
                {t('disconnect')}
              </Button>
            </div>
          )}
        </>
      )}
      <ConfirmDialog
        open={disconnectOpen}
        onOpenChange={setDisconnectOpen}
        entity={setup.taxId || null}
        title={t('disconnectTitle')}
        description={(cuit) => t('disconnectDescription', { cuit })}
        onConfirm={disconnect}
        pending={disconnecting}
        labels={{ confirm: t('disconnect'), pending: t('disconnecting'), cancel: t('cancel') }}
      />
    </div>
  );
}
