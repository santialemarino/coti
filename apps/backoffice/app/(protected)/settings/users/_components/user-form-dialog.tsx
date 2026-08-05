'use client';

import { useEffect, useId, useMemo, useRef } from 'react';
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
  Input,
  Label,
  PendingButton,
  RadioGroup,
  RadioGroupItem,
} from '@repo/ui/components';
import type { UserResult } from '@/app/(protected)/settings/users/actions';
import {
  userSchema,
  type UserFormMode,
  type UserValues,
} from '@/app/(protected)/settings/users/form-schema';
import type { Branch } from '@/lib/api/branches';
import type { AccountUser } from '@/lib/api/users';
import {
  ADMIN_ROLE,
  PASSWORD_MAX_LENGTH,
  PASSWORD_MIN_LENGTH,
  SELLER_ROLE,
  USER_ROLES,
  type UserRole,
} from '@/lib/constants/auth';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

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
  onSubmit,
}: UserFormDialogProps) {
  const t = useTranslations('users');
  const tCommon = useTranslations('common');
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

  const schema = useMemo(() => userSchema(shown.mode, t), [shown.mode, t]);
  const form = useForm<UserValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', email: '', role: SELLER_ROLE, branchIds: [], password: '' },
  });
  const role = useWatch({ control: form.control, name: 'role' });
  const pending = form.formState.isSubmitting;

  /*
   * Reset on open, not on mount: the dialog outlives every user it edits, so without this the
   * second row opened would still be showing the first row's values.
   */
  useEffect(() => {
    if (!open) return;
    form.reset({
      name: user?.name ?? '',
      email: user?.email ?? '',
      role: roleOf(user?.role),
      branchIds: assigned.map((branch) => branch.id),
      password: '',
    });
  }, [open, user, assigned, form]);

  async function submit(values: UserValues) {
    const result = await onSubmit(values);
    // The address is the one rejection that belongs to a field, so it reads like a validation error
    // in the place the caller has to fix.
    if (result.error === 'emailTaken') {
      form.setError('email', { message: t('errors.emailTaken') });
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !next && !pending && onOpenChange(false)}>
      <DialogContent className="sm:max-w-md" closeOnClickOutside={!pending}>
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

            {shown.mode === 'create' ? (
              <FormField
                control={form.control}
                name="password"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel required>{t('password.label')}</FormLabel>
                    <FormControl>
                      <Input
                        type="password"
                        autoComplete="new-password"
                        minLength={PASSWORD_MIN_LENGTH}
                        maxLength={PASSWORD_MAX_LENGTH}
                        placeholder={t('password.placeholder')}
                        passwordToggleLabel={tCommon('form.togglePassword')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t('password.hint', { count: PASSWORD_MIN_LENGTH })}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
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

// The wire carries a plain string. An unknown role resolves to the narrower of the two, so a value
// the interface cannot render can never widen someone's reach.
function roleOf(raw: string | undefined): UserRole {
  return raw === ADMIN_ROLE ? ADMIN_ROLE : SELLER_ROLE;
}
