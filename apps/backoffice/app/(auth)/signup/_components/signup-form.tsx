'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useTranslations } from 'next-intl';
import { useForm, type FieldErrors, type Resolver } from 'react-hook-form';

import {
  Button,
  Form,
  FormRootMessage,
  InlineLink,
  PendingButton,
  Stepper,
} from '@repo/ui/components';
import { AuthCard } from '@/app/(auth)/_components/auth-card';
import { AccountStep } from '@/app/(auth)/signup/_components/account-step';
import { AdminStep } from '@/app/(auth)/signup/_components/admin-step';
import { BranchStep } from '@/app/(auth)/signup/_components/branch-step';
import { signup } from '@/app/(auth)/signup/actions';
import { type SignupValues } from '@/app/(auth)/signup/form-schema';
import { STEP_ORDER, stepOwning, STEPS, stepSchema, type StepKey } from '@/app/(auth)/signup/steps';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { FORM_VALIDATION } from '@/lib/forms/options';

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
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('auth.signup');
  const text = useMemo(() => ({ field: t, shared: tErrors }), [t, tErrors]);
  const [stepKey, setStepKey] = useState<StepKey>('account');
  /* Read by the resolver, which runs outside a render and needs the step as of the submit. */
  const currentStep = useRef<StepKey>('account');
  /*
   * Whether the caller has tried to leave the step they are on. Advancing marks the whole form
   * submitted, and react-hook-form re-checks every change from then on — so without this the last
   * step starts reporting errors on the first character typed into a field nobody has submitted.
   */
  const stepSubmitted = useRef(false);

  /*
   * The step decides what a submit validates, so the schema changes under the form and
   * `zodResolver` — built around one fixed schema — is not what runs here. The values pass through
   * untouched, since a step's schema covers only its own fields.
   */
  const resolver: Resolver<SignupValues> = useCallback(
    async (values) => {
      if (!stepSubmitted.current) return { values, errors: {} };

      const parsed = await stepSchema(currentStep.current, text).safeParseAsync(values);
      if (parsed.success) return { values, errors: {} };

      const errors: Record<string, { type: string; message: string }> = {};
      for (const issue of parsed.error.issues) {
        // Every field of the wizard is a flat key, so the first path segment names it.
        const name = String(issue.path[0]);
        errors[name] ??= { type: issue.code, message: issue.message };
      }
      return { values: {}, errors: errors as FieldErrors<SignupValues> };
    },
    [text],
  );

  const form = useForm<SignupValues>({
    ...FORM_VALIDATION,
    resolver,
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
  const navigated = useRef(false);

  /*
   * Unmounting the step the caller was on drops focus to the body, so without this tabbing would
   * restart from the top of the page on every step and a screen reader would be told nothing.
   * Whichever field carries an error is the one to act on, so a rejection lands on it directly.
   * Never on the first render: taking focus before anyone has interacted skips past the heading
   * that says what the screen is.
   */
  useEffect(() => {
    if (!navigated.current) return;
    const fields = STEPS[stepKey].fields;
    const target = fields.find((name) => form.getFieldState(name).error) ?? fields[0];
    if (target) form.setFocus(target);
  }, [stepKey, form]);

  function goToStep(next: StepKey) {
    navigated.current = true;
    currentStep.current = next;
    stepSubmitted.current = false;
    setStepKey(next);
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
    if (result.field) {
      /*
       * The step has to move with the error. Nothing ties the wizard's position to the form's
       * state, so a message set on a field the caller cannot see makes the button look dead —
       * and stepping back while the request is in flight is enough to be somewhere else when
       * this lands.
       */
      const owner = stepOwning(result.field);
      if (owner) goToStep(owner);
    }
    form.setError(result.field ?? 'root', { message: message(result.error) });
  }

  function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    // Set before either branch, because both reach the resolver, which reads it.
    stepSubmitted.current = true;

    /*
     * The primary button submits on every step so Enter does what pressing it does; which of the two
     * things that means is decided here. Advancing goes through `handleSubmit` rather than
     * `trigger` — only a submit puts the form in the state where a rejected field re-checks itself
     * on every keystroke.
     */
    const { next } = step;
    if (next) {
      void form.handleSubmit(() => goToStep(next))(event);
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
            {/*
              Keyed so the fields remount and replay the entrance, and so the swap stays atomic:
              the stepper, the description and the button all sit outside this box, and fields that
              outlive a step change put one step's inputs under the next step's button — where a
              click submits a step nobody has filled in.
            */}
            <div key={stepKey} className="flex flex-col gap-y-5 animate-rise-in">
              <Fields />
            </div>

            <FormRootMessage />

            <div className="flex items-center gap-x-3">
              {previous ? (
                <Button
                  type="button"
                  variant="outline"
                  size="lg"
                  onClick={() => goToStep(previous)}
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
