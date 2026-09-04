'use client';

import { useMemo, useState, useTransition } from 'react';
import { KeyRoundIcon, PencilIcon, PlusIcon, RotateCcwIcon, UserXIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Callout,
  ConfirmDialog,
  RowActionButton,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@repo/ui/components';
import {
  createUser,
  deactivateUser,
  reactivateUser,
  sendPasswordReset,
  updateUser,
  type UserResult,
} from '@/app/(protected)/settings/users/actions';
import type { UserFormMode, UserValues } from '@/app/(protected)/settings/users/form-schema';
import { UserFormDialog } from '@/components/user-form-dialog';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import type { AccountUser } from '@/lib/api/users';
import { ADMIN_ROLE } from '@/lib/constants/auth';
import { useFormatters } from '@/lib/i18n/formatters';

// Stable, because the dialog resets itself when its assignments change: a fresh empty array would
// wipe what the caller had typed the next time anything re-rendered the page under them.
const NO_BRANCHES: Branch[] = [];

interface UserRow {
  user: AccountUser;
  /*
   * The active branches they hold. A branch closed after the assignment was made stays in the
   * database but the API refuses to accept it back, so it is neither listed nor re-sent.
   */
  assigned: Branch[];
}

interface UserTableProps {
  users: AccountUser[];
  /* The caller's branch reach — active branches only, which is all the API will assign. */
  branches: Branch[];
  currentUserId: string;
}

export function UserTable({ users, branches, currentUserId }: UserTableProps) {
  const fmt = useFormatters();
  const t = useTranslations('users');
  const tCommon = useTranslations('common');
  /*
   * Two resolvers because mailing a link words two codes differently from the rest of the flow:
   * a 422 there means the user is deactivated, and a 429 that the mail allowance is spent.
   */
  const message = useApiErrorMessage('users');
  const resetMessage = useApiErrorMessage('users.passwordReset');
  const [form, setForm] = useState<{ mode: UserFormMode; row: UserRow | null } | null>(null);
  const [deactivating, setDeactivating] = useState<UserRow | null>(null);
  const [resetting, setResetting] = useState<UserRow | null>(null);
  const [error, setError] = useState<string | null>(null);
  /*
   * One transition per action, never one shared: a shared transition only reports that something is
   * running, so mailing a recovery link would light the button that deactivates. Saving needs none —
   * the form dialog reports its own submission.
   */
  const [removing, startRemove] = useTransition();
  const [reactivating, startReactivate] = useTransition();
  const [mailing, startMail] = useTransition();
  const busy = removing || reactivating || mailing;
  const rows = useMemo<UserRow[]>(
    () =>
      users.map((user) => ({
        user,
        assigned: branches.filter((branch) => user.branchIds.includes(branch.id)),
      })),
    [users, branches],
  );

  async function onSubmit(values: UserValues): Promise<UserResult> {
    const target = form;
    if (!target) return { error: 'INTERNAL' };
    setError(null);
    const result =
      target.mode === 'edit' && target.row
        ? await updateUser(target.row.user.id, values)
        : await createUser(values);
    if (result.ok) {
      // A confirmation of something just done is transient, so it is a toast; the standing message
      // about what is on screen is the Callout above.
      toast.success(t(target.mode === 'edit' ? 'updated' : 'created', { name: values.name }));
      setForm(null);
      return result;
    }
    // Every rejection but one belongs to the list. The address belongs to its field, and the dialog
    // is what puts it there.
    if (result.error !== 'EMAIL_TAKEN') setError(message(result.error));
    return result;
  }

  function onDeactivate() {
    const target = deactivating;
    if (!target) return;
    setError(null);
    startRemove(async () => {
      const result = await deactivateUser(target.user.id);
      if (!result.ok) {
        // Closed either way: the refusal belongs to the list, not to a dialog that is about to
        // disappear.
        setDeactivating(null);
        setError(message(result.error));
        return;
      }
      toast.success(t('deactivated', { name: target.user.name }));
      setDeactivating(null);
    });
  }

  function onReactivate(row: UserRow) {
    setError(null);
    startReactivate(async () => {
      const result = await reactivateUser({
        id: row.user.id,
        name: row.user.name,
        email: row.user.email,
        role: row.user.role,
        // The active ones only: a branch closed since the assignment was made would make the API
        // refuse the whole replacement, so reactivating leaves it behind.
        branchIds: row.assigned.map((branch) => branch.id),
      });
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      toast.success(t('reactivated', { name: row.user.name }));
    });
  }

  function onSendPasswordReset() {
    const target = resetting;
    if (!target) return;
    setError(null);
    startMail(async () => {
      const result = await sendPasswordReset(target.user.id);
      if (!result.ok) {
        setResetting(null);
        setError(resetMessage(result.error));
        return;
      }
      toast.success(t('passwordResetSent', { email: target.user.email }));
      setResetting(null);
    });
  }

  return (
    <div className="flex flex-col gap-y-6">
      {error ? <Callout tone="danger">{error}</Callout> : null}

      <div className="flex justify-end">
        <Button disabled={busy} onClick={() => setForm({ mode: 'create', row: null })}>
          <PlusIcon aria-hidden="true" />
          {t('add')}
        </Button>
      </div>

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead>{t('table.name')}</TableHead>
            <TableHead>{t('table.email')}</TableHead>
            <TableHead>{t('table.role')}</TableHead>
            <TableHead>{t('table.branches')}</TableHead>
            <TableHead>{t('table.status')}</TableHead>
            <TableHead>{t('table.lastLogin')}</TableHead>
            <TableHead className="text-right">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => {
            const { user, assigned } = row;
            const isSelf = user.id === currentUserId;
            const reach =
              user.role === ADMIN_ROLE
                ? t('table.allBranches')
                : assigned.length > 0
                  ? fmt.list(assigned.map((branch) => branch.name))
                  : null;

            return (
              <TableRow key={user.id}>
                <TableCell className="text-paragraph-sm-medium text-foreground">
                  <span className="flex items-center gap-x-2">
                    {user.name}
                    {/* The action this row does not offer is explained here, because a disabled
                        control cannot fire the tooltip that would say why. */}
                    {isSelf ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Badge
                            tone="brand"
                            size="sm"
                            tabIndex={0}
                            className="outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45 cursor-default"
                          >
                            {t('you')}
                          </Badge>
                        </TooltipTrigger>
                        <TooltipContent>{t('yourUser')}</TooltipContent>
                      </Tooltip>
                    ) : null}
                  </span>
                </TableCell>
                <TableCell>{user.email}</TableCell>
                <TableCell>{tCommon(`roles.${user.role}`)}</TableCell>
                <TableCell className={reach ? undefined : 'text-foreground-subtle'}>
                  {reach ?? t('table.noBranches')}
                </TableCell>
                <TableCell>
                  <Badge tone={user.isActive ? 'success' : 'neutral'}>
                    {t(user.isActive ? 'status.active' : 'status.inactive')}
                  </Badge>
                </TableCell>
                <TableCell className={user.lastLoginAt ? undefined : 'text-foreground-subtle'}>
                  {user.lastLoginAt ? fmt.date(user.lastLoginAt) : t('table.neverLoggedIn')}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-x-1">
                    <RowActionButton
                      icon={PencilIcon}
                      label={t('edit.action')}
                      disabled={busy}
                      onClick={() => setForm({ mode: 'edit', row })}
                    />
                    {user.isActive ? (
                      <RowActionButton
                        icon={KeyRoundIcon}
                        label={t('passwordReset.action')}
                        disabled={busy}
                        onClick={() => setResetting(row)}
                      />
                    ) : null}
                    {user.isActive && !isSelf ? (
                      <RowActionButton
                        icon={UserXIcon}
                        label={t('deactivate.action')}
                        tone="danger"
                        disabled={busy}
                        onClick={() => setDeactivating(row)}
                      />
                    ) : null}
                    {user.isActive ? null : (
                      <RowActionButton
                        icon={RotateCcwIcon}
                        label={t('reactivate.action')}
                        disabled={busy}
                        onClick={() => onReactivate(row)}
                      />
                    )}
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>

      <UserFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && setForm(null)}
        mode={form?.mode ?? 'create'}
        user={form?.row?.user ?? null}
        assigned={form?.row?.assigned ?? NO_BRANCHES}
        branches={branches}
        isSelf={form?.row?.user.id === currentUserId}
        onSubmit={onSubmit}
      />

      <ConfirmDialog
        open={deactivating !== null}
        onOpenChange={(open) => !open && !removing && setDeactivating(null)}
        entity={deactivating}
        title={t('deactivate.title')}
        description={(row: UserRow) => t('deactivate.description', { name: row.user.name })}
        onConfirm={onDeactivate}
        pending={removing}
        labels={{
          confirm: t('deactivate.confirm'),
          pending: t('deactivate.confirming'),
          cancel: t('cancel'),
        }}
      />

      <ConfirmDialog
        open={resetting !== null}
        onOpenChange={(open) => !open && !mailing && setResetting(null)}
        entity={resetting}
        title={t('passwordReset.title')}
        description={(row: UserRow) => t('passwordReset.description', { email: row.user.email })}
        onConfirm={onSendPasswordReset}
        pending={mailing}
        tone="default"
        labels={{
          confirm: t('passwordReset.confirm'),
          pending: t('passwordReset.confirming'),
          cancel: t('cancel'),
        }}
      />
    </div>
  );
}
