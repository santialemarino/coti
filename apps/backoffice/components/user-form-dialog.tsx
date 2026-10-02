'use client';

import { useId, useLayoutEffect, useMemo, useRef } from 'react';
import Link from 'next/link';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslations } from 'next-intl';
import { useForm, useWatch } from 'react-hook-form';

import {
  Button,
  Callout,
  Checkbox,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  Hint,
  InlineLink,
  Input,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
} from '@repo/ui/components';
import type { UserResult } from '@/app/(protected)/settings/users/actions';
import {
  USER_ACCESS,
  userSchema,
  type UserAccess,
  type UserFormMode,
  type UserValues,
} from '@/app/(protected)/settings/users/form-schema';
import { PasswordField } from '@/components/password-field';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import type { AccountUser } from '@/lib/api/users';
import { ADMIN_ROLE, SELLER_ROLE, USER_ROLES, type UserRole } from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';
import { FORM_VALIDATION } from '@/lib/forms/options';

interface UserFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /* Which copy the dialog wears. Explicit, because a null user is what opening one looks like. */
  mode: UserFormMode;
  /* The user being edited. May go null while the dialog animates out. */
  user: AccountUser | null;
  /* The active branches they hold — the only ones an update may send back. */
  assigned: Branch[];
  branches: Branch[];
  /* True when the caller is editing themselves, which the API refuses to let change their role. */
  isSelf: boolean;
  /* False while mail only reaches the log, when an invite could never be redeemed. */
  mailDelivery: boolean;
  onSubmit: (values: UserValues) => Promise<UserResult>;
}

/*
 * One dialog for both creating and editing a user: the fields, their validation and the request
 * body are the same, and only the copy, the initial password and the target differ.
 */
export function UserFormDialog({
  open,
  onOpenChange,
  mode,
  user,
  assigned,
  branches,
  isSelf,
  mailDelivery,
  onSubmit,
}: UserFormDialogProps) {
  const t = useTranslations('users');
  const tCommon = useTranslations('common');
  const tErrors = useTranslations('common.form.errors');
  const message = useApiErrorMessage('users');
  const fieldId = useId();
  /*
   * An assignment to a branch that has since closed cannot be sent back — the API only accepts
   * active ones — so saving drops it. Said out loud, because it is data the caller did not ask to
   * lose and the checkbox group cannot show.
   */
  const closedAssignments = (user?.branchIds.length ?? 0) - assigned.length;
  /*
   * What the dialog renders is snapshotted while open and held afterwards, the way `ConfirmDialog`
   * holds its entity: the caller clears its selection on close, and a dialog that relabels itself,
   * grows a password field or drops a warning while it fades looks broken.
   */
  const lastShown = useRef({ mode, isSelf, closedAssignments });
  if (open) lastShown.current = { mode, isSelf, closedAssignments };
  const shown = open ? { mode, isSelf, closedAssignments } : lastShown.current;

  const schema = useMemo(
    () => userSchema(shown.mode, { field: t, shared: tErrors }),
    [shown.mode, t, tErrors],
  );
  const form = useForm<UserValues>({
    ...FORM_VALIDATION,
    resolver: zodResolver(schema),
    defaultValues: {
      name: '',
      email: '',
      role: SELLER_ROLE,
      branchIds: [],
      access: initialAccess(mailDelivery),
      password: '',
    },
  });
  const role = useWatch({ control: form.control, name: 'role' });
  const access = useWatch({ control: form.control, name: 'access' });
  const email = useWatch({ control: form.control, name: 'email' });
  // A new user in an account with one branch has one obvious assignment, so it is made. An id, not
  // the list: a re-render hands over a fresh array, and resetting on it would wipe what was typed.
  const soleBranchId = mode === 'create' && branches.length === 1 ? branches[0]?.id : undefined;
  const pending = form.formState.isSubmitting;

  /*
   * Reset on open, not on mount: the dialog outlives every user it edits, so without this the
   * second row opened would still be showing the first row's values.
   */
  useLayoutEffect(() => {
    if (!open) return;
    form.reset({
      name: user?.name ?? '',
      email: user?.email ?? '',
      role: roleOf(user?.role),
      branchIds: soleBranchId ? [soleBranchId] : assigned.map((branch) => branch.id),
      access: initialAccess(mailDelivery),
      password: '',
    });
  }, [open, user, assigned, soleBranchId, mailDelivery, form]);

  async function submit(values: UserValues) {
    const result = await onSubmit(values);
    // The address is the one rejection that belongs to a field, so it reads like a validation error
    // in the place the caller has to fix.
    if (result.error === 'EMAIL_TAKEN') {
      form.setError('email', { message: message(result.error) });
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !pending && onOpenChange(false)}>
      <DialogContent className="sm:max-w-lg" closeOnClickOutside={!pending} showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>{t(`${shown.mode}.title`)}</DialogTitle>
          <DialogDescription>{t(`${shown.mode}.description`)}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(submit)} noValidate className="flex flex-col gap-y-5">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel required>{t('name.label')}</FormLabel>
                  <FormControl>
                    <Input
                      autoComplete="name"
                      maxLength={TEXT_FIELD_MAX_LENGTH}
                      placeholder={t('name.placeholder')}
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Their own address changes where the password is asked for again, never here. */}
            {shown.isSelf ? (
              <div className="flex flex-col gap-y-1">
                <span className="text-paragraph-sm-medium text-foreground">{t('email.label')}</span>
                <p className="text-paragraph-sm text-foreground">{email}</p>
                <Hint>
                  {t('email.ownAddress')}{' '}
                  <InlineLink asChild tone="muted">
                    <Link href={ROUTES.emailSettings}>{t('email.ownAddressLink')}</Link>
                  </InlineLink>
                </Hint>
              </div>
            ) : (
              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('email.label')}</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        autoComplete="email"
                        maxLength={TEXT_FIELD_MAX_LENGTH}
                        placeholder={t('email.placeholder')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {shown.mode === 'create' && mailDelivery ? (
              <FormField
                control={form.control}
                name="access"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('access.label')}</FormLabel>
                    <FormControl>
                      <RadioGroup
                        aria-label={t('access.label')}
                        className="gap-y-3"
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        {USER_ACCESS.map((option) => (
                          <div key={option} className="flex items-start gap-x-2.5">
                            <RadioGroupItem
                              id={`${fieldId}-access-${option}`}
                              value={option}
                              className="mt-0.5"
                            />
                            <div className="flex flex-col gap-y-0.5">
                              <Label
                                htmlFor={`${fieldId}-access-${option}`}
                                className="cursor-pointer"
                              >
                                {t(`access.${option}.label`)}
                              </Label>
                              <Hint>{t(`access.${option}.hint`)}</Hint>
                            </div>
                          </div>
                        ))}
                      </RadioGroup>
                    </FormControl>
                  </FormItem>
                )}
              />
            ) : null}

            {shown.mode === 'create' && access === 'PASSWORD' ? (
              <div className="flex flex-col gap-y-2">
                <PasswordField
                  control={form.control}
                  name="password"
                  label={t('password.label')}
                  placeholder={t('password.placeholder')}
                  meter
                />
                {/* With no mail there is one way in, so it is stated rather than offered. */}
                {mailDelivery ? null : <Hint>{t('access.passwordOnly')}</Hint>}
              </div>
            ) : null}

            {/* The explanation stands where the control would have been. */}
            {shown.isSelf ? (
              <Callout>{t('yourUser')}</Callout>
            ) : (
              <FormField
                control={form.control}
                name="role"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('role.label')}</FormLabel>
                    <FormControl>
                      {/* The group names itself: `for` does not associate a label with a div. */}
                      <RadioGroup
                        aria-label={t('role.label')}
                        className="gap-y-3"
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        {USER_ROLES.map((option) => (
                          <div key={option} className="flex items-start gap-x-2.5">
                            <RadioGroupItem
                              id={`${fieldId}-role-${option}`}
                              value={option}
                              className="mt-0.5"
                            />
                            <div className="flex flex-col gap-y-0.5">
                              <Label
                                htmlFor={`${fieldId}-role-${option}`}
                                className="cursor-pointer"
                              >
                                {tCommon(`roles.${option}`)}
                              </Label>
                              <Hint>{t(`roleHints.${option}`)}</Hint>
                            </div>
                          </div>
                        ))}
                      </RadioGroup>
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            <FormField
              control={form.control}
              name="branchIds"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('branches.label')}</FormLabel>
                  <FormControl>
                    <div
                      role="group"
                      aria-label={t('branches.label')}
                      className="flex flex-col gap-y-2.5"
                    >
                      {branches.map((branch) => (
                        <div key={branch.id} className="flex items-center gap-x-2.5">
                          <Checkbox
                            id={`${fieldId}-branch-${branch.id}`}
                            checked={field.value.includes(branch.id)}
                            onCheckedChange={(checked) =>
                              field.onChange(
                                checked === true
                                  ? [...field.value, branch.id]
                                  : field.value.filter((id) => id !== branch.id),
                              )
                            }
                          />
                          <Label
                            htmlFor={`${fieldId}-branch-${branch.id}`}
                            className="cursor-pointer"
                          >
                            {branch.name}
                          </Label>
                        </div>
                      ))}
                    </div>
                  </FormControl>
                  <FormDescription>
                    {role === ADMIN_ROLE ? t('branches.adminHint') : t('branches.hint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {shown.closedAssignments > 0 ? (
              <Callout tone="warning">
                {t('branches.closedAssignments', { count: shown.closedAssignments })}
              </Callout>
            ) : null}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={pending}
                onClick={() => onOpenChange(false)}
              >
                {t('cancel')}
              </Button>
              <PendingButton
                type="submit"
                pending={pending}
                pendingLabel={t(`${shown.mode}.submitting`)}
              >
                {t(`${shown.mode}.submit`)}
              </PendingButton>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

// An invite is the default whenever one can be delivered; otherwise a password is the only way in.
function initialAccess(mailDelivery: boolean): UserAccess {
  return mailDelivery ? 'INVITE' : 'PASSWORD';
}

// The wire carries a plain string. An unknown role resolves to the narrower of the two, so a value
// the interface cannot render can never widen someone's reach.
function roleOf(raw: string | undefined): UserRole {
  return raw === ADMIN_ROLE ? ADMIN_ROLE : SELLER_ROLE;
}
