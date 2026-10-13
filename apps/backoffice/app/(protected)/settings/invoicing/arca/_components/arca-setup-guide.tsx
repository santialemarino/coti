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
import { PointsOfSaleForm } from '@/app/(protected)/settings/invoicing/arca/_components/points-of-sale-form';
import { createARCASetup, disconnectARCA } from '@/app/(protected)/settings/invoicing/arca/actions';
import {
  ARCA_STEPS,
  arcaStepStatuses,
  STEP_ANCHORS,
  type ARCAStep,
} from '@/app/(protected)/settings/invoicing/arca/step-statuses';
import { SetupNotice } from '@/components/setup-notice';
import { ARCA_URLS } from '@/config/arca';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { ARCASetup } from '@/lib/api/arca-setup';
import { formatCuit, type InvoicingBranch } from '@/lib/api/invoicing';
import { useFormatters } from '@/lib/i18n/formatters';
import type { ARCAConnectionState } from '@/lib/utils/arca-connection';
import { isValidCuit } from '@/lib/utils/cuit';

// Removing the credential means starting over while it is in progress, and disconnecting after.
type RemoveMode = 'restart' | 'disconnect';

// The states the guide calls out above its steps, as the status card does.
const SUMMARY_TONE: Partial<Record<ARCAConnectionState, 'warning' | 'danger'>> = {
  expired: 'danger',
  pointsMissing: 'warning',
  expiringSoon: 'warning',
};

interface ARCASetupGuideProps {
  setup: ARCASetup;
  // The account's CUIT: the one the request is generated for, edited only in Cuenta.
  accountTaxId: string | null;
  branches: InvoicingBranch[];
  state: ARCAConnectionState;
}

export function ARCASetupGuide({ setup, accountTaxId, branches, state }: ARCASetupGuideProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('arca');
  const tConnection = useTranslations('invoicing.settings.connection');
  const message = useApiErrorMessage('arca');
  const [generating, startGenerating] = useTransition();
  const [removeOpen, setRemoveOpen] = useState(false);
  const [removing, startRemoving] = useTransition();

  const statuses = arcaStepStatuses(setup, branches);
  // The request can only be generated for a CUIT ARCA accepts; anything else is fixed in Cuenta.
  const usableCuit = accountTaxId !== null && isValidCuit(accountTaxId);
  const cuit = setup.taxId || accountTaxId || '';
  const shownCuit = cuit ? formatCuit(cuit.replace(/\D/g, '')) : '';
  const expiresOn = setup.certificateExpiresAt ? fmt.date(setup.certificateExpiresAt) : '';
  const activeBranches = branches.filter((branch) => branch.isActive);
  const busy = generating || removing;
  const mode: RemoveMode = setup.hasCertificate ? 'disconnect' : 'restart';
  // The dialog keeps what it was about while it fades out after the credential is gone.
  const confirm = useHeldWhileClosed({ mode, cuit: shownCuit }, removeOpen);
  const summaryTone = SUMMARY_TONE[state];

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

  function remove() {
    startRemoving(async () => {
      const result = await disconnectARCA();
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      setRemoveOpen(false);
      toast.success(t(`remove.${mode}.done`));
      router.refresh();
    });
  }

  const content: Record<ARCAStep, React.ReactNode> = {
    wsass: (
      <>
        <Paragraph>{t('steps.wsass.body')}</Paragraph>
        <Paragraph>{t('steps.wsass.check', { lookup: t('wsassLookup') })}</Paragraph>
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
            <figure className="flex flex-col gap-y-1.5">
              <figcaption className="text-paragraph-xs-medium text-foreground-muted">
                {t('steps.copy.label')}
              </figcaption>
              <ScrollArea className="max-h-48 bg-sunken border border-border rounded-lg">
                <pre className="px-3 py-2 text-paragraph-xs font-mono text-foreground whitespace-pre-wrap break-all">
                  {setup.csr}
                </pre>
              </ScrollArea>
            </figure>
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
        <PemExample instruction={t('pem.instruction')} />
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
          {expiresOn
            ? t(state === 'expired' ? 'steps.upload.expired' : 'steps.upload.installed', {
                date: expiresOn,
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
        {setup.hasCertificate ? (
          <>
            <Paragraph>{t('steps.pointsOfSale.body')}</Paragraph>
            <Paragraph>
              {t('steps.pointsOfSale.progress', {
                done: activeBranches.filter((branch) => branch.pointOfSale !== null).length,
                total: activeBranches.length,
              })}
            </Paragraph>
            <PointsOfSaleForm branches={branches} disabled={busy} />
          </>
        ) : (
          <Paragraph>{t('steps.pointsOfSale.pending')}</Paragraph>
        )}
      </>
    ),
  };

  if (!setup.enabled) return <Callout tone="warning">{t('disabled')}</Callout>;

  const steps: StepGuideStep[] = ARCA_STEPS.map((step) => ({
    id: step,
    anchor: STEP_ANCHORS[step],
    title: t(`steps.${step}.title`),
    status: statuses[step],
    children: content[step],
  }));

  return (
    <div className="flex flex-col max-w-2xl gap-y-8">
      {summaryTone ? (
        <Callout tone={summaryTone} title={tConnection(`states.${state}`)}>
          {tConnection(`bodies.${state}`, { date: expiresOn })}
        </Callout>
      ) : null}
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
          <p className="text-paragraph-sm text-foreground-muted">{t(`remove.${mode}.hint`)}</p>
          <Button variant="outline" onClick={() => setRemoveOpen(true)} disabled={busy}>
            {t(`remove.${mode}.action`)}
          </Button>
        </div>
      ) : null}
      <ConfirmDialog
        open={removeOpen}
        onOpenChange={setRemoveOpen}
        entity={confirm.cuit || null}
        title={t(`remove.${confirm.mode}.title`)}
        description={(value) => t(`remove.${confirm.mode}.description`, { cuit: value })}
        onConfirm={remove}
        pending={removing}
        labels={{
          confirm: t(`remove.${confirm.mode}.action`),
          pending: t(`remove.${confirm.mode}.pending`),
          cancel: t('cancel'),
        }}
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
