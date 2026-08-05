'use client';

import { useMemo, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm } from 'react-hook-form';

import {
  Button,
  Form,
  FormRootMessage,
  InlineLink,
  PendingButton,
  Stepper,
} from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { AuthStage } from '@/app/(auth)/_components/auth-stage';
import { AccountStep } from '@/app/(auth)/signup/_components/account-step';
import { AdminStep } from '@/app/(auth)/signup/_components/admin-step';
import { BranchStep } from '@/app/(auth)/signup/_components/branch-step';
import { signup } from '@/app/(auth)/signup/actions';
import { signupSchema, type SignupValues } from '@/app/(auth)/signup/form-schema';
import { STEP_ORDER, stepOwning, STEPS, type StepKey } from '@/app/(auth)/signup/steps';
import { ROUTES } from '@/config/routes';

const STEP_FIELDS: Record<StepKey, () => React.ReactNode> = {
  account: AccountStep,
  branch: BranchStep,
  admin: AdminStep,
};

const EMPTY_VALUES: SignupValues = {
  accountName: '',
  legalName: '',
  taxId: '',
  branchName: '',
  branchAddress: '',
  adminName: '',
  adminEmail: '',
  adminPassword: '',
  confirmPassword: '',
};

/*
 * One form across three steps, not three forms: the account, its first branch and its
 * administrator are created by a single request, so a caller who abandons the wizard has
 * created nothing.
 */
export function SignupForm() {
  const router = useRouter();
  const t = useTranslations('auth.signup');
  const schema = useMemo(() => signupSchema(t), [t]);
  const [stepKey, setStepKey] = useState<StepKey>('account');
  const form = useForm<SignupValues>({
    resolver: zodResolver(schema),
    defaultValues: EMPTY_VALUES,
  });

  const labels = useMemo(
    () => STEP_ORDER.map((key) => ({ id: key, label: t(`steps.${key}.label`) })),
    [t],
  );
  const step = STEPS[stepKey];
  const previous = step.previous;
  const Fields = STEP_FIELDS[stepKey];
  const submitting = form.formState.isSubmitting;

  async function advance(next: StepKey) {
    // This step's fields only. Validating the whole form here would mark fields the caller has
    // not reached, leaving messages on steps nobody is looking at.
    if (await form.trigger(step.fields)) setStepKey(next);
  }

  async function create(values: SignupValues) {
    const result = await signup(values);
    if (result.redirectTo) {
      router.replace(result.redirectTo);
      // The session cookie only exists as of this response, so the cached tree still
      // reflects an anonymous caller.
      router.refresh();
      return;
    }
    if (result.fieldError) {
      /*
       * The step has to move with the error. Nothing ties the wizard's position to the form's
       * state, so a message set on a field the caller cannot see makes the button look dead —
       * and stepping back while the request is in flight is enough to be somewhere else when
       * this lands.
       */
      const owner = stepOwning(result.fieldError.field);
      if (owner) setStepKey(owner);
      form.setError(result.fieldError.field, { message: t(`errors.${result.fieldError.key}`) });
      return;
    }
    form.setError('root', { message: t(`errors.${result.error ?? 'unexpected'}`) });
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    // The primary button submits on every step so Enter does what pressing it does; which of
    // the two things that means is decided here.
    if (step.next) {
      void advance(step.next);
      return;
    }
    // A disabled button stops a second click, not a second submit — and this is the one
    // request that would open a second account.
    if (submitting) return;
    void form.handleSubmit(create)(event);
  }

  return (
    <AuthCard
      title={t('title')}
      description={t(`steps.${stepKey}.description`)}
      footer={
        <InlineLink asChild tone="muted">
          <Link href={ROUTES.login}>{t('haveAccount')}</Link>
        </InlineLink>
      }
    >
      <div className="flex flex-col gap-y-6">
        <Stepper steps={labels} currentIndex={STEP_ORDER.indexOf(stepKey)} />

        <Form {...form}>
          <form onSubmit={onSubmit} noValidate className="flex flex-col gap-y-5">
            <AuthStage stageKey={stepKey}>
              <div className="flex flex-col gap-y-5">
                <Fields />
              </div>
            </AuthStage>

            <FormRootMessage />

            <div className="flex items-center gap-x-3">
              {previous ? (
                <Button
                  type="button"
                  variant="outline"
                  size="lg"
                  onClick={() => setStepKey(previous)}
                >
                  {t('back')}
                </Button>
              ) : null}

              {step.next ? (
                <Button type="submit" size="lg" className="flex-1">
                  {t('next')}
                </Button>
              ) : (
                <PendingButton
                  type="submit"
                  size="lg"
                  className="flex-1"
                  pending={submitting}
                  pendingLabel={t('submitting')}
                >
                  {t('submit')}
                </PendingButton>
              )}
            </div>
          </form>
        </Form>
      </div>
    </AuthCard>
  );
}
