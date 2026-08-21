'use client';

import { useRef, useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  CheckCircle2Icon,
  FileSpreadsheetIcon,
  PaletteIcon,
  StoreIcon,
  UsersIcon,
} from 'lucide-react';
import { AnimatePresence, motion, useReducedMotion } from 'motion/react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Card,
  ConfirmDialog,
  PendingButton,
  StatusScreen,
} from '@repo/ui/components';
import { MOTION } from '@repo/ui/lib';
import { BranchStep } from '@/app/(onboarding)/onboarding/_components/branch-step';
import { BrandStep } from '@/app/(onboarding)/onboarding/_components/brand-step';
import { OnboardingProgress } from '@/app/(onboarding)/onboarding/_components/onboarding-progress';
import { TeamStep } from '@/app/(onboarding)/onboarding/_components/team-step';
import {
  completeOnboarding,
  createOnboardingUser,
  dismissOnboarding,
  resumeOnboarding,
  saveOnboardingStep,
  updateOnboardingBranch,
  updateOnboardingBrand,
} from '@/app/(onboarding)/onboarding/actions';
import type { OnboardingBrandValues } from '@/app/(onboarding)/onboarding/form-schema';
import { resumeStep } from '@/app/(onboarding)/onboarding/steps';
import type { CatalogImportPreview } from '@/app/(protected)/_actions/catalog-import';
import type { BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import type { UserValues } from '@/app/(protected)/settings/users/form-schema';
import { CatalogReview, CatalogUpload } from '@/components/catalog-import';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Account } from '@/lib/api/account';
import type { Branch } from '@/lib/api/branches';
import type { Onboarding, OnboardingStepKey, OnboardingStepStatus } from '@/lib/api/onboarding';
import type { AccountUser } from '@/lib/api/users';

const BRAND_FORM_ID = 'onboarding-brand-form';
const BRANCH_FORM_ID = 'onboarding-branch-form';
const CATALOG_UPLOAD_FORM_ID = 'onboarding-catalog-upload-form';

interface OnboardingFlowProps {
  onboarding: Onboarding;
  account: Account;
  branch: Branch;
  branches: Branch[];
  users: AccountUser[];
  currentUserId: string;
}

export function OnboardingFlow({
  onboarding,
  account,
  branch,
  branches,
  users,
  currentUserId,
}: OnboardingFlowProps) {
  const router = useRouter();
  const t = useTranslations('onboarding');
  const tCatalog = useTranslations('catalogImport');
  const message = useApiErrorMessage('onboarding');
  const reduced = useReducedMotion();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const firstRender = useRef(true);
  const [status, setStatus] = useState(onboarding.status);
  const [step, setStep] = useState<OnboardingStepKey>(
    onboarding.status === 'COMPLETED' ? 'COMPLETE' : resumeStep(onboarding.currentStep),
  );
  const [resolved, setResolved] = useState(onboarding.steps);
  const [preview, setPreview] = useState<CatalogImportPreview | null>(null);
  const [catalogBusy, setCatalogBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dismissOpen, setDismissOpen] = useState(false);
  const [pending, startTransition] = useTransition();
  const currentStepRef = useRef(step);
  currentStepRef.current = step;

  function move(next: OnboardingStepKey) {
    setError(null);
    setStep(next);
  }

  async function resolveStep(
    completedStep: OnboardingStepKey,
    stepStatus: OnboardingStepStatus,
    next: OnboardingStepKey,
  ): Promise<boolean> {
    const result = await saveOnboardingStep(completedStep, stepStatus, next);
    if (!result.ok) {
      setError(message(result.error));
      return false;
    }
    setResolved((current) => ({ ...current, [completedStep]: stepStatus }));
    move(next);
    return true;
  }

  function run(task: () => Promise<void>) {
    setError(null);
    startTransition(task);
  }

  function submitBrand(values: OnboardingBrandValues) {
    run(async () => {
      const result = await updateOnboardingBrand(values);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      await resolveStep('BRAND', 'COMPLETED', 'FIRST_BRANCH');
    });
  }

  function submitBranch(values: BranchValues) {
    run(async () => {
      const result = await updateOnboardingBranch(branch.id, values);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      await resolveStep('FIRST_BRANCH', 'COMPLETED', 'CATALOG_UPLOAD');
    });
  }

  async function previewCatalog(nextPreview: CatalogImportPreview) {
    setError(null);
    if (await resolveStep('CATALOG_UPLOAD', 'COMPLETED', 'CATALOG_REVIEW')) {
      setPreview(nextPreview);
    }
  }

  function skipCatalog() {
    run(async () => {
      const upload = await saveOnboardingStep('CATALOG_UPLOAD', 'SKIPPED', 'CATALOG_REVIEW');
      if (!upload.ok) {
        setError(message(upload.error));
        return;
      }
      setResolved((current) => ({ ...current, CATALOG_UPLOAD: 'SKIPPED' }));
      await resolveStep('CATALOG_REVIEW', 'SKIPPED', 'TEAM');
    });
  }

  function finishTeam(stepStatus: OnboardingStepStatus) {
    run(async () => {
      if (!(await resolveStep('TEAM', stepStatus, 'COMPLETE'))) return;
      const result = await completeOnboarding();
      if (!result.ok) {
        setError(message(result.error));
        move('TEAM');
      }
    });
  }

  function dismiss() {
    run(async () => {
      const result = await dismissOnboarding();
      if (!result.ok) {
        setDismissOpen(false);
        setError(message(result.error));
        return;
      }
      router.replace(ROUTES.home);
      router.refresh();
    });
  }

  function resume() {
    run(async () => {
      const result = await resumeOnboarding();
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      setStatus('IN_PROGRESS');
      move(resumeStep(onboarding.currentStep));
    });
  }

  if (status === 'DISMISSED') {
    return (
      <main className="flex min-h-screen items-center justify-center px-6 py-12">
        <Card className="max-w-xl items-center p-8 gap-y-6 text-center">
          <span className="flex size-12 items-center justify-center bg-accent rounded-full text-accent-foreground">
            <StoreIcon aria-hidden="true" className="size-6" />
          </span>
          <div className="flex flex-col gap-y-2">
            <h1 className="text-heading-3">{t('dismissed.title')}</h1>
            <p className="text-paragraph text-foreground-muted">{t('dismissed.description')}</p>
          </div>
          {error ? <Callout tone="danger">{error}</Callout> : null}
          <div className="flex flex-wrap justify-center gap-3">
            <Button asChild variant="outline">
              <Link href={ROUTES.home}>{t('dismissed.home')}</Link>
            </Button>
            <PendingButton
              pending={pending}
              pendingLabel={t('dismissed.resuming')}
              onClick={resume}
            >
              {t('dismissed.resume')}
            </PendingButton>
          </div>
        </Card>
      </main>
    );
  }

  const screen = screenCopy(step);

  return (
    <main className="mx-auto flex min-h-screen w-full max-w-5xl flex-col px-5 py-6 gap-y-8 sm:px-8 sm:py-10">
      <header className="flex items-center justify-between gap-x-4">
        <Link href={ROUTES.home} className="text-heading-4 focus-visible:focus-ring rounded-sm">
          Coti
        </Link>
        {step !== 'COMPLETE' ? (
          <Button
            type="button"
            variant="outline"
            disabled={pending}
            onClick={() => setDismissOpen(true)}
          >
            {t('skipAll.action')}
          </Button>
        ) : null}
      </header>

      <OnboardingProgress current={step} resolved={resolved} />

      <Card className="animate-rise-in flex-1 p-5 gap-y-0 sm:p-9">
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={step}
            className="flex min-h-full flex-1 flex-col gap-y-7"
            initial={{ opacity: 0, x: reduced ? 0 : 12 }}
            animate={{ opacity: 1, x: 0 }}
            exit={{ opacity: 0, x: reduced ? 0 : -12 }}
            transition={{ duration: reduced ? 0 : MOTION.slow }}
            onAnimationComplete={() => {
              if (currentStepRef.current !== step) return;
              if (firstRender.current) {
                firstRender.current = false;
                return;
              }
              headingRef.current?.focus();
            }}
          >
            <header className="flex flex-col gap-y-2">
              <h1 ref={headingRef} tabIndex={-1} className="text-heading-2 outline-none">
                {t(`${screen}.title`)}
              </h1>
              <p className="max-w-3xl text-paragraph text-foreground-muted">
                {t(`${screen}.description`)}
              </p>
            </header>

            {error ? <Callout tone="danger">{error}</Callout> : null}
            {renderStep()}
          </motion.div>
        </AnimatePresence>
      </Card>

      <ConfirmDialog
        open={dismissOpen}
        onOpenChange={(open) => !pending && setDismissOpen(open)}
        entity={true}
        title={t('skipAll.title')}
        description={() => t('skipAll.description')}
        onConfirm={dismiss}
        pending={pending}
        labels={{
          confirm: t('skipAll.confirm'),
          pending: t('skipAll.dismissing'),
          cancel: t('skipAll.cancel'),
        }}
      />
    </main>
  );

  function renderStep() {
    switch (step) {
      case 'WELCOME':
        return (
          <>
            <div className="grid gap-3 md:grid-cols-2">
              {(['brand', 'branch', 'catalog', 'team'] as const).map((item) => (
                <Card
                  key={item}
                  className="flex-row items-start p-4 gap-y-0 gap-x-3 bg-muted rounded-lg shadow-e1"
                >
                  <CheckCircle2Icon
                    aria-hidden="true"
                    className="mt-0.5 size-5 shrink-0 text-primary"
                  />
                  <div className="flex flex-col gap-y-1">
                    <p className="text-paragraph-sm-medium">
                      {t(`welcome.checklist.${item}.title`)}
                    </p>
                    <p className="text-paragraph-xs text-foreground-muted">
                      {t(`welcome.checklist.${item}.description`)}
                    </p>
                  </div>
                </Card>
              ))}
            </div>
            <Footer
              pending={pending}
              primary={t('welcome.start')}
              pendingLabel={t('saving')}
              onPrimary={() =>
                run(async () => void (await resolveStep('WELCOME', 'COMPLETED', 'BRAND')))
              }
            />
          </>
        );
      case 'BRAND':
        return (
          <>
            <BrandStep account={account} formId={BRAND_FORM_ID} onSubmit={submitBrand} />
            <Footer
              form={BRAND_FORM_ID}
              pending={pending}
              primary={t('continue')}
              pendingLabel={t('saving')}
            />
          </>
        );
      case 'FIRST_BRANCH':
        return (
          <>
            <BranchStep branch={branch} formId={BRANCH_FORM_ID} onSubmit={submitBranch} />
            <Footer
              form={BRANCH_FORM_ID}
              pending={pending}
              primary={t('continue')}
              pendingLabel={t('saving')}
              back={() => move('BRAND')}
            />
          </>
        );
      case 'CATALOG_UPLOAD':
        return (
          <>
            <CatalogUpload
              branch={branch}
              formId={CATALOG_UPLOAD_FORM_ID}
              submitPlacement="external"
              onBusyChange={setCatalogBusy}
              onPreview={previewCatalog}
            />
            <Footer
              form={CATALOG_UPLOAD_FORM_ID}
              pending={pending}
              disabled={pending || catalogBusy}
              primary={t('continue')}
              primaryPending={catalogBusy}
              pendingLabel={tCatalog('previewing')}
              secondary={t('catalog.later')}
              secondaryPending={pending}
              secondaryPendingLabel={t('saving')}
              back={() => move('FIRST_BRANCH')}
              onSecondary={skipCatalog}
            />
          </>
        );
      case 'CATALOG_REVIEW':
        return preview ? (
          <CatalogReview
            preview={preview}
            onBack={() => move('CATALOG_UPLOAD')}
            onConfirmed={() => {
              run(async () => void (await resolveStep('CATALOG_REVIEW', 'COMPLETED', 'TEAM')));
            }}
          />
        ) : (
          <Button
            type="button"
            variant="outline"
            className="self-start"
            onClick={() => move('CATALOG_UPLOAD')}
          >
            {t('review.returnToUpload')}
          </Button>
        );
      case 'TEAM':
        return (
          <>
            <TeamStep
              branches={branches}
              currentUserId={currentUserId}
              users={users}
              onCreate={(values: UserValues) => createOnboardingUser(values)}
            />
            <Footer
              pending={pending}
              primary={t('team.finish')}
              pendingLabel={t('saving')}
              back={() => move('CATALOG_UPLOAD')}
              onPrimary={() => finishTeam('COMPLETED')}
            />
          </>
        );
      case 'COMPLETE':
        return (
          <div className="flex flex-1 flex-col gap-y-8">
            <StatusScreen
              icon={CheckCircle2Icon}
              tone="success"
              title={t('complete.readyTitle')}
              description={t('complete.readyDescription')}
            />
            <div className="grid w-full max-w-3xl gap-4 sm:grid-cols-3">
              <ReadyItem icon={PaletteIcon} text={t('complete.items.brand')} />
              <ReadyItem icon={StoreIcon} text={t('complete.items.branch')} />
              <ReadyItem icon={FileSpreadsheetIcon} text={t('complete.items.catalog')} />
            </div>
            <Button asChild size="lg" className="mt-auto w-full sm:ml-auto sm:w-auto">
              <Link href={ROUTES.home}>{t('complete.action')}</Link>
            </Button>
          </div>
        );
      default:
        return null;
    }
  }
}

interface FooterProps {
  form?: string;
  pending: boolean;
  disabled?: boolean;
  primary?: string;
  primaryPending?: boolean;
  pendingLabel?: string;
  primaryVariant?: 'default' | 'outline';
  back?: () => void;
  secondary?: string;
  secondaryPending?: boolean;
  secondaryPendingLabel?: string;
  onPrimary?: () => void;
  onSecondary?: () => void;
}

function Footer({
  form,
  pending,
  disabled = pending,
  primary,
  primaryPending = pending,
  pendingLabel,
  primaryVariant = 'default',
  back,
  secondary,
  secondaryPending = pending,
  secondaryPendingLabel,
  onPrimary,
  onSecondary,
}: FooterProps) {
  const t = useTranslations('onboarding');
  return (
    <footer className="mt-auto flex flex-col items-stretch pt-5 gap-3 sm:flex-row sm:items-center sm:justify-between sm:pt-7">
      <div className="contents sm:block">
        {back ? (
          <Button
            type="button"
            variant="outline"
            className="w-full sm:w-auto"
            disabled={disabled}
            onClick={back}
          >
            {t('back')}
          </Button>
        ) : null}
      </div>
      <div className="flex flex-col items-stretch gap-2 sm:flex-row sm:items-center">
        {secondary && secondaryPendingLabel && onSecondary ? (
          <PendingButton
            type="button"
            variant="ghost"
            className="w-full sm:w-auto"
            disabled={disabled}
            pending={secondaryPending}
            pendingLabel={secondaryPendingLabel}
            onClick={onSecondary}
          >
            {secondary}
          </PendingButton>
        ) : null}
        {primary && pendingLabel ? (
          <PendingButton
            type={form ? 'submit' : 'button'}
            form={form}
            variant={primaryVariant}
            className="w-full sm:w-auto"
            disabled={disabled}
            pending={primaryPending}
            pendingLabel={pendingLabel}
            onClick={form ? undefined : onPrimary}
          >
            {primary}
          </PendingButton>
        ) : null}
      </div>
    </footer>
  );
}

function ReadyItem({ icon: Icon, text }: { icon: typeof UsersIcon; text: string }) {
  return (
    <Card className="min-h-36 items-center justify-center p-6 gap-y-4 bg-muted shadow-e1">
      <Icon aria-hidden="true" className="size-8 text-primary" />
      <span className="text-heading-6">{text}</span>
    </Card>
  );
}

function screenCopy(step: OnboardingStepKey): string {
  switch (step) {
    case 'FIRST_BRANCH':
      return 'branch';
    case 'CATALOG_UPLOAD':
      return 'catalog';
    case 'CATALOG_REVIEW':
      return 'review';
    default:
      return step.toLowerCase();
  }
}
