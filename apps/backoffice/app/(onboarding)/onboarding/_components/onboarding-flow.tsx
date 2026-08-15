'use client';

import { useEffect, useRef, useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import {
  CheckCircle2Icon,
  FileSpreadsheetIcon,
  PaletteIcon,
  StoreIcon,
  UsersIcon,
} from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout, ConfirmDialog, PendingButton } from '@repo/ui/components';
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
  const message = useApiErrorMessage('onboarding');
  const headingRef = useRef<HTMLHeadingElement>(null);
  const firstRender = useRef(true);
  const [status, setStatus] = useState(onboarding.status);
  const [step, setStep] = useState<OnboardingStepKey>(
    onboarding.status === 'COMPLETED' ? 'COMPLETE' : resumeStep(onboarding.currentStep),
  );
  const [resolved, setResolved] = useState(onboarding.steps);
  const [preview, setPreview] = useState<CatalogImportPreview | null>(null);
  const [catalogResult, setCatalogResult] = useState<{ imported: number; skipped: number } | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);
  const [dismissOpen, setDismissOpen] = useState(false);
  const [pending, startTransition] = useTransition();

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false;
      return;
    }
    headingRef.current?.focus();
  }, [step]);

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

  function previewCatalog(nextPreview: CatalogImportPreview) {
    run(async () => {
      if (await resolveStep('CATALOG_UPLOAD', 'COMPLETED', 'CATALOG_REVIEW')) {
        setPreview(nextPreview);
      }
    });
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
        <section className="flex w-full max-w-xl flex-col items-center p-8 gap-y-6 bg-card border rounded-1.5xl shadow-e2 text-center">
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
        </section>
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

      <section className="animate-rise-in flex flex-1 flex-col p-6 gap-y-7 bg-card border rounded-1.5xl shadow-e2 sm:p-9">
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
      </section>

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
                <div key={item} className="flex items-start p-4 gap-x-3 bg-muted border rounded-lg">
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
                </div>
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
            <CatalogUpload branch={branch} onPreview={previewCatalog} />
            <Footer
              pending={pending}
              primary={t('catalog.later')}
              pendingLabel={t('saving')}
              primaryVariant="outline"
              back={() => move('FIRST_BRANCH')}
              onPrimary={skipCatalog}
            />
          </>
        );
      case 'CATALOG_REVIEW':
        return preview ? (
          <CatalogReview
            preview={preview}
            onBack={() => move('CATALOG_UPLOAD')}
            onConfirmed={(imported, skipped) => {
              setCatalogResult({ imported, skipped });
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
            {catalogResult ? (
              <Callout tone="success">{t('team.catalogImported', catalogResult)}</Callout>
            ) : null}
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
              skip={t('skipStep')}
              back={() => move('CATALOG_UPLOAD')}
              onPrimary={() => finishTeam('COMPLETED')}
              onSkip={() => finishTeam('SKIPPED')}
            />
          </>
        );
      case 'COMPLETE':
        return (
          <div className="flex flex-1 flex-col items-center justify-center py-8 gap-y-8 text-center">
            <div className="grid w-full max-w-3xl gap-4 sm:grid-cols-3">
              <ReadyItem icon={PaletteIcon} text={t('complete.items.brand')} />
              <ReadyItem icon={StoreIcon} text={t('complete.items.branch')} />
              <ReadyItem icon={FileSpreadsheetIcon} text={t('complete.items.catalog')} />
            </div>
            <Button asChild size="lg">
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
  primary: string;
  pendingLabel: string;
  primaryVariant?: 'default' | 'outline';
  back?: () => void;
  skip?: string;
  onPrimary?: () => void;
  onSkip?: () => void;
}

function Footer({
  form,
  pending,
  primary,
  pendingLabel,
  primaryVariant = 'default',
  back,
  skip,
  onPrimary,
  onSkip,
}: FooterProps) {
  const t = useTranslations('onboarding');
  return (
    <footer className="flex flex-wrap items-center justify-between pt-2 gap-3 border-t">
      <div>
        {back ? (
          <Button type="button" variant="outline" disabled={pending} onClick={back}>
            {t('back')}
          </Button>
        ) : null}
      </div>
      <div className="flex items-center gap-x-2">
        {skip && onSkip ? (
          <Button type="button" variant="ghost" disabled={pending} onClick={onSkip}>
            {skip}
          </Button>
        ) : null}
        <PendingButton
          type={form ? 'submit' : 'button'}
          form={form}
          variant={primaryVariant}
          pending={pending}
          pendingLabel={pendingLabel}
          onClick={form ? undefined : onPrimary}
        >
          {primary}
        </PendingButton>
      </div>
    </footer>
  );
}

function ReadyItem({ icon: Icon, text }: { icon: typeof UsersIcon; text: string }) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center p-6 gap-y-4 bg-muted border rounded-1.5xl">
      <Icon aria-hidden="true" className="size-8 text-primary" />
      <span className="text-heading-6">{text}</span>
    </div>
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
