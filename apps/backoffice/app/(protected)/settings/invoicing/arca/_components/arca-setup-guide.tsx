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
import {
  createARCASetup,
  disconnectARCA,
  reconnectARCA,
} from '@/app/(protected)/settings/invoicing/arca/actions';
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

/*
 * Removing the credential means starting over while it is in progress and disconnecting after; a
 * connection made for the other ARCA environment is replaced by a new request in one step.
 */
type RemoveMode = 'restart' | 'disconnect' | 'reconnect';

// What ARCA shows on the way to the certificate, which differs between its two environments.
const CERTIFICATE_ITEMS = {
  HOMOLOGATION: ['open', 'alias', 'paste', 'result'],
  PRODUCTION: ['open', 'alias', 'upload', 'view'],
} as const;

// The steps whose title names what ARCA calls its own screens, which differ between environments.
const STEP_TITLES_BY_ENVIRONMENT: readonly ARCAStep[] = ['wsass', 'copy', 'certificate'];

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

  const env = setup.environment;
  const reconnecting = state === 'reconnect';
  // A connection made for the other environment is started over, so its steps read from the top.
  const current = reconnecting ? { ...setup, taxId: '', csr: '', hasCertificate: false } : setup;
  const statuses = arcaStepStatuses(current, branches);
  // The request can only be generated for a CUIT ARCA accepts; anything else is fixed in Cuenta.
  const usableCuit = accountTaxId !== null && isValidCuit(accountTaxId);
  const cuit = current.taxId || accountTaxId || '';
  const shownCuit = cuit ? formatCuit(cuit.replace(/\D/g, '')) : '';
  const removedCuit = formatCuit((setup.taxId || accountTaxId || '').replace(/\D/g, ''));
  const expiresOn = setup.certificateExpiresAt ? fmt.date(setup.certificateExpiresAt) : '';
  const activeBranches = branches.filter((branch) => branch.isActive);
  const busy = generating || removing;
  const mode: RemoveMode = reconnecting
    ? 'reconnect'
    : setup.hasCertificate
      ? 'disconnect'
      : 'restart';
  // The dialog keeps what it was about while it fades out after the credential is gone.
  const confirm = useHeldWhileClosed({ mode, cuit: removedCuit }, removeOpen);
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
      const result = await (mode === 'reconnect' ? reconnectARCA() : disconnectARCA());
      if (result.error) {
        toast.error(message(result.error));
        return;
      }
      setRemoveOpen(false);
      toast.success(t(`remove.${mode}.done`));
      router.refresh();
    });
  }

  function copyRequest(variant: 'default' | 'outline') {
    return (
      <CopyButton
        variant={variant}
        size="default"
        value={setup.csr}
        labels={{ copy: t('steps.copy.copy'), copied: t('steps.copy.copied') }}
        onCopyError={() => toast.error(t('steps.copy.copyError'))}
      />
    );
  }

  function downloadRequest(variant: 'default' | 'outline') {
    return (
      <Button
        variant={variant}
        onClick={() => download(setup.csr, `coti-${cuit}.csr`, 'application/pkcs10')}
      >
        <DownloadIcon aria-hidden="true" />
        {t('steps.copy.download')}
      </Button>
    );
  }

  const content: Record<ARCAStep, React.ReactNode> = {
    wsass: (
      <>
        <Paragraph>{t(`steps.wsass.${env}.body`)}</Paragraph>
        <Paragraph>
          {env === 'HOMOLOGATION'
            ? t('steps.wsass.HOMOLOGATION.check', { lookup: t('wsassLookup') })
            : t('steps.wsass.PRODUCTION.check')}
        </Paragraph>
        <ExternalButton href={ARCA_URLS.login}>{t('steps.wsass.open')}</ExternalButton>
      </>
    ),
    generate: (
      <>
        <Paragraph>{t('steps.generate.body')}</Paragraph>
        {usableCuit || current.csr ? (
          <dl className="flex flex-col gap-y-1">
            <dt className="text-paragraph-xs-medium text-foreground-muted">
              {t('steps.generate.cuit')}
            </dt>
            <dd className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span className="text-paragraph-semibold text-foreground tabular-nums">
                {shownCuit}
              </span>
              {current.csr ? null : (
                <InlineLink asChild tone="muted">
                  <Link href={ROUTES.accountSettings}>{t('steps.generate.editCuit')}</Link>
                </InlineLink>
              )}
            </dd>
          </dl>
        ) : (
          <SetupNotice issue="ACCOUNT_TAX_ID" isAdmin />
        )}
        {current.csr ? (
          <Paragraph>{t('steps.generate.locked')}</Paragraph>
        ) : usableCuit ? (
          <PendingButton
            className="self-start"
            pending={generating}
            pendingLabel={t('steps.generate.submitting')}
            disabled={busy}
            onClick={reconnecting ? () => setRemoveOpen(true) : generate}
          >
            {t('steps.generate.submit')}
          </PendingButton>
        ) : null}
      </>
    ),
    copy: (
      <>
        <Paragraph>{t(`steps.copy.${env}.body`)}</Paragraph>
        {current.csr ? (
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
            {/* WSASS takes the request pasted; production ARCA takes it as a file. */}
            <div className="flex flex-wrap gap-2">
              {env === 'PRODUCTION' ? (
                <>
                  {downloadRequest('default')}
                  {copyRequest('outline')}
                </>
              ) : (
                <>
                  {copyRequest('default')}
                  {downloadRequest('outline')}
                </>
              )}
            </div>
          </>
        ) : null}
      </>
    ),
    certificate: (
      <>
        <ol className="flex flex-col pl-5 gap-y-2 list-decimal text-paragraph-sm text-foreground-muted">
          {CERTIFICATE_ITEMS[env].map((item) => (
            <li key={item}>{t(`steps.certificate.${env}.${item}`)}</li>
          ))}
        </ol>
        <PemExample instruction={t('pem.instruction')} />
        {env === 'PRODUCTION' ? (
          <ExternalButton href={ARCA_URLS.certificateGuide}>
            {t('steps.certificate.guide')}
          </ExternalButton>
        ) : null}
      </>
    ),
    authorize: (
      <>
        <Paragraph>
          {t.rich(`steps.authorize.${env}.body`, {
            cuit: () => <span className="whitespace-nowrap">{shownCuit || '—'}</span>,
          })}
        </Paragraph>
        <ExternalButton href={ARCA_URLS.authorizationGuide[env]}>
          {t('steps.authorize.guide')}
        </ExternalButton>
      </>
    ),
    upload: current.hasCertificate ? (
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
        <Paragraph>{t(`steps.upload.${env}.body`)}</Paragraph>
        <CertificateUpload disabled={!current.csr || busy} filePrimary={env === 'PRODUCTION'} />
      </>
    ),
    pointsOfSale: (
      <>
        {current.hasCertificate ? (
          <>
            <Paragraph>{t(`steps.pointsOfSale.${env}.body`)}</Paragraph>
            <Paragraph>
              {t('steps.pointsOfSale.progress', {
                done: activeBranches.filter((branch) => branch.pointOfSale !== null).length,
                total: activeBranches.length,
              })}
            </Paragraph>
            <PointsOfSaleForm branches={branches} disabled={busy} />
          </>
        ) : (
          <Paragraph>{t(`steps.pointsOfSale.${env}.pending`)}</Paragraph>
        )}
      </>
    ),
  };

  if (!setup.enabled) return <Callout tone="warning">{t('disabled')}</Callout>;

  const steps: StepGuideStep[] = ARCA_STEPS.map((step) => ({
    id: step,
    anchor: STEP_ANCHORS[step],
    title: t(
      STEP_TITLES_BY_ENVIRONMENT.includes(step)
        ? `steps.${step}.${env}.title`
        : `steps.${step}.title`,
    ),
    status: statuses[step],
    children: content[step],
  }));

  return (
    <div className="flex flex-col max-w-2xl gap-y-8">
      {reconnecting ? (
        <Callout tone="warning" title={t('reconnect.title')}>
          {t('reconnect.body')}
        </Callout>
      ) : summaryTone ? (
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
      {setup.csr && !reconnecting ? (
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
