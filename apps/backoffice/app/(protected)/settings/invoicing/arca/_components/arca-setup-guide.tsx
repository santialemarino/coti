'use client';

import { useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { DownloadIcon, ExternalLinkIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Button,
  Callout,
  ConfirmDialog,
  CopyButton,
  InlineLink,
  PendingButton,
  ScrollArea,
  StepGuide,
  type StepGuideStep,
} from '@repo/ui/components';
import { useHeldWhileClosed } from '@repo/ui/hooks';
import { CertificateUpload } from '@/app/(protected)/settings/invoicing/arca/_components/certificate-upload';
import { PemExample } from '@/app/(protected)/settings/invoicing/arca/_components/pem-example';
import { createARCASetup, disconnectARCA } from '@/app/(protected)/settings/invoicing/arca/actions';
import {
  ARCA_STEPS,
  arcaStepStatuses,
  type ARCAStep,
} from '@/app/(protected)/settings/invoicing/arca/step-statuses';
import { SetupNotice } from '@/components/setup-notice';
import { ARCA_URLS } from '@/config/arca';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ARCASetup } from '@/lib/api/arca-setup';
import { formatCuit, type InvoicingBranch } from '@/lib/api/invoicing';
import { useFormatters } from '@/lib/i18n/formatters';
import { isValidCuit } from '@/lib/utils/cuit';

interface ARCASetupGuideProps {
  setup: ARCASetup;
  // The account's CUIT: the one the request is generated for, edited only in Cuenta.
  accountTaxId: string | null;
  branches: InvoicingBranch[];
  now: string;
}

export function ARCASetupGuide({ setup, accountTaxId, branches, now }: ARCASetupGuideProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('arca');
  const message = useApiErrorMessage('arca');
  const [generating, startGenerating] = useTransition();
  const [disconnectOpen, setDisconnectOpen] = useState(false);
  const [disconnecting, startDisconnecting] = useTransition();

  const statuses = arcaStepStatuses(setup, branches);
  // The request can only be generated for a CUIT ARCA accepts; anything else is fixed in Cuenta.
  const usableCuit = accountTaxId !== null && isValidCuit(accountTaxId);
  const cuit = setup.taxId || accountTaxId || '';
  const shownCuit = cuit ? formatCuit(cuit.replace(/\D/g, '')) : '';
  const expired = setup.certificateExpiresAt !== null && setup.certificateExpiresAt < now;
  const activeBranches = branches.filter((branch) => branch.isActive);
  const busy = generating || disconnecting;
  // The CUIT is gone once the disconnect lands, while the dialog is still fading out.
  const confirmCuit = useHeldWhileClosed(shownCuit, disconnectOpen);

  function generate() {
    startGenerating(async () => {
      const result = await createARCASetup();
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      toast.success(t('steps.generate.generated'));
      router.refresh();
    });
  }

  function disconnect() {
    startDisconnecting(async () => {
      const result = await disconnectARCA();
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      setDisconnectOpen(false);
      toast.success(t('disconnected'));
      router.refresh();
    });
  }

  const content: Record<ARCAStep, React.ReactNode> = {
    wsass: (
      <>
        <Paragraph>{t('steps.wsass.body')}</Paragraph>
        <ExternalButton href={ARCA_URLS.login}>{t('steps.wsass.open')}</ExternalButton>
      </>
    ),
    generate: (
      <>
        <Paragraph>{t('steps.generate.body')}</Paragraph>
        {usableCuit || setup.csr ? (
          <dl className="flex flex-col gap-y-1">
            <dt className="text-paragraph-xs-medium text-foreground-muted">
              {t('steps.generate.cuit')}
            </dt>
            <dd className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span className="text-paragraph-semibold text-foreground tabular-nums">
                {shownCuit}
              </span>
              {setup.csr ? null : (
                <InlineLink asChild tone="muted">
                  <Link href={ROUTES.accountSettings}>{t('steps.generate.editCuit')}</Link>
                </InlineLink>
              )}
            </dd>
          </dl>
        ) : (
          <SetupNotice issue="ACCOUNT_TAX_ID" isAdmin />
        )}
        {setup.csr ? (
          <Paragraph>{t('steps.generate.locked')}</Paragraph>
        ) : usableCuit ? (
          <PendingButton
            className="self-start"
            pending={generating}
            pendingLabel={t('steps.generate.submitting')}
            disabled={busy}
            onClick={generate}
          >
            {t('steps.generate.submit')}
          </PendingButton>
        ) : null}
      </>
    ),
    copy: (
      <>
        <Paragraph>{t('steps.copy.body')}</Paragraph>
        {setup.csr ? (
          <>
            <ScrollArea className="max-h-48 bg-sunken border border-border rounded-lg">
              <pre
                aria-label={t('steps.copy.label')}
                className="px-3 py-2 text-paragraph-xs font-mono text-foreground whitespace-pre-wrap break-all"
              >
                {setup.csr}
              </pre>
            </ScrollArea>
            <div className="flex flex-wrap gap-2">
              <CopyButton
                variant="default"
                size="default"
                value={setup.csr}
                labels={{ copy: t('steps.copy.copy'), copied: t('steps.copy.copied') }}
                onCopyError={() => toast.error(t('steps.copy.copyError'))}
              />
              <Button
                variant="outline"
                onClick={() => download(setup.csr, `coti-${cuit}.csr`, 'application/pkcs10')}
              >
                <DownloadIcon aria-hidden="true" />
                {t('steps.copy.download')}
              </Button>
            </div>
          </>
        ) : null}
      </>
    ),
    certificate: (
      <>
        <ol className="flex flex-col pl-5 gap-y-2 list-decimal text-paragraph-sm text-foreground-muted">
          <li>{t('steps.certificate.open')}</li>
          <li>{t('steps.certificate.alias')}</li>
          <li>{t('steps.certificate.paste')}</li>
          <li>{t('steps.certificate.result')}</li>
        </ol>
        <PemExample label="CERTIFICATE" caption={t('pemCaption')} />
      </>
    ),
    authorize: (
      <>
        <Paragraph>
          {t.rich('steps.authorize.body', {
            cuit: () => <span className="whitespace-nowrap">{shownCuit || '—'}</span>,
          })}
        </Paragraph>
        <ExternalButton href={ARCA_URLS.authorizationGuide}>
          {t('steps.authorize.guide')}
        </ExternalButton>
      </>
    ),
    upload: setup.hasCertificate ? (
      <>
        <Paragraph>
          {setup.certificateExpiresAt
            ? t(expired ? 'steps.upload.expired' : 'steps.upload.installed', {
                date: fmt.date(setup.certificateExpiresAt),
              })
            : t('steps.upload.installedUndated')}
        </Paragraph>
        <Button
          variant="outline"
          className="self-start"
          onClick={() => download(setup.certificate, `coti-${cuit}.pem`, 'application/x-pem-file')}
        >
          <DownloadIcon aria-hidden="true" />
          {t('steps.upload.download')}
        </Button>
      </>
    ) : (
      <>
        <Paragraph>{t('steps.upload.body')}</Paragraph>
        <CertificateUpload disabled={!setup.csr || busy} />
      </>
    ),
    pointsOfSale: (
      <>
        <Paragraph>{t('steps.pointsOfSale.body')}</Paragraph>
        {setup.hasCertificate ? (
          <Paragraph>
            {t('steps.pointsOfSale.progress', {
              done: activeBranches.filter((branch) => branch.pointOfSale !== null).length,
              total: activeBranches.length,
            })}
          </Paragraph>
        ) : null}
        <Button asChild variant="outline" className="self-start">
          <Link href={ROUTES.invoicingSection('points-of-sale')}>
            {t('steps.pointsOfSale.link')}
          </Link>
        </Button>
      </>
    ),
  };

  if (!setup.enabled) return <Callout tone="warning">{t('disabled')}</Callout>;

  const steps: StepGuideStep[] = ARCA_STEPS.map((step) => ({
    id: step,
    title: t(`steps.${step}.title`),
    status: statuses[step],
    children: content[step],
  }));

  return (
    <div className="flex flex-col max-w-2xl gap-y-8">
      <StepGuide
        steps={steps}
        statusLabels={{
          done: t('statuses.done'),
          current: t('statuses.current'),
          pending: t('statuses.pending'),
        }}
      />
      {setup.csr ? (
        <div className="flex flex-col items-start pt-6 gap-y-2 border-t border-border">
          <p className="text-paragraph-sm text-foreground-muted">{t('disconnectHint')}</p>
          <Button variant="outline" onClick={() => setDisconnectOpen(true)} disabled={busy}>
            {t('disconnect')}
          </Button>
        </div>
      ) : null}
      <ConfirmDialog
        open={disconnectOpen}
        onOpenChange={setDisconnectOpen}
        entity={confirmCuit || null}
        title={t('disconnectTitle')}
        description={(value) => t('disconnectDescription', { cuit: value })}
        onConfirm={disconnect}
        pending={disconnecting}
        labels={{ confirm: t('disconnect'), pending: t('disconnecting'), cancel: t('cancel') }}
      />
    </div>
  );
}

function Paragraph({ children }: { children: React.ReactNode }) {
  return <p className="text-paragraph-sm text-foreground-muted">{children}</p>;
}

function ExternalButton({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <Button asChild variant="outline" className="self-start">
      <a href={href} target="_blank" rel="noopener noreferrer">
        {children}
        <ExternalLinkIcon aria-hidden="true" />
      </a>
    </Button>
  );
}

function download(content: string, fileName: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  link.click();
  URL.revokeObjectURL(url);
}
