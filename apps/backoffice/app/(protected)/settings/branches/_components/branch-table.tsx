'use client';

import { useState, useTransition } from 'react';
import { BuildingIcon, PencilIcon, PlusIcon, RotateCcwIcon, XCircleIcon } from 'lucide-react';
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
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableRow,
} from '@repo/ui/components';
import { BranchFormDialog } from '@/app/(protected)/settings/branches/_components/branch-form-dialog';
import {
  closeBranch,
  createBranch,
  reopenBranch,
  updateBranch,
} from '@/app/(protected)/settings/branches/actions';
import type { BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { missesBranchEmail } from '@/lib/utils/setup-issues';

const COLUMN_COUNT = 5;

interface BranchTableProps {
  branches: Branch[];
}

export function BranchTable({ branches }: BranchTableProps) {
  const t = useTranslations('branches');
  const message = useApiErrorMessage('branches');
  const [form, setForm] = useState<{ mode: 'create' | 'edit'; branch: Branch | null } | null>(null);
  const [closing, setClosing] = useState<Branch | null>(null);
  const [error, setError] = useState<string | null>(null);
  /*
   * One transition per action, never one shared: a shared transition only reports that something
   * is running, so closing a branch would light the dialog that saves one.
   */
  const [saving, startSave] = useTransition();
  const [removing, startRemove] = useTransition();
  const [reopening, startReopen] = useTransition();
  const busy = saving || removing || reopening;

  function onSubmit(values: BranchValues) {
    const target = form;
    if (!target) return;
    setError(null);
    startSave(async () => {
      const result =
        target.mode === 'edit' && target.branch
          ? await updateBranch(target.branch.id, values)
          : await createBranch(values);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      // A confirmation of something just done is transient, so it is a toast; the standing
      // message about what is on screen is the Callout above.
      toast.success(t(target.mode === 'edit' ? 'updated' : 'created', { name: values.name }));
      setForm(null);
    });
  }

  function onClose() {
    const target = closing;
    if (!target) return;
    setError(null);
    startRemove(async () => {
      const result = await closeBranch(target.id);
      if (!result.ok) {
        // Closed either way: the refusal belongs to the list, not to a dialog that is about to
        // disappear, and the account needing one active branch is a fact about the whole account.
        setClosing(null);
        setError(message(result.error));
        return;
      }
      toast.success(t('closed', { name: target.name }));
      setClosing(null);
    });
  }

  function onReopen(branch: Branch) {
    setError(null);
    startReopen(async () => {
      const result = await reopenBranch(branch);
      if (!result.ok) {
        setError(message(result.error));
        return;
      }
      toast.success(t('reopened', { name: branch.name }));
    });
  }

  return (
    <div className="flex flex-col gap-y-6">
      {error ? <Callout tone="danger">{error}</Callout> : null}

      <div className="flex justify-end">
        <Button disabled={busy} onClick={() => setForm({ mode: 'create', branch: null })}>
          <PlusIcon aria-hidden="true" />
          {t('add')}
        </Button>
      </div>

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead kind="text">{t('table.name')}</TableHead>
            <TableHead kind="text">{t('table.address')}</TableHead>
            <TableHead kind="count">{t('table.expiry')}</TableHead>
            <TableHead kind="status">{t('table.status')}</TableHead>
            <TableHead kind="actionsWide">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {branches.length === 0 ? (
            <TableEmptyRow
              colSpan={COLUMN_COUNT}
              icon={BuildingIcon}
              title={t('empty.title')}
              description={t('empty.description')}
            />
          ) : (
            branches.map((branch) => (
              <TableRow key={branch.id}>
                <TableCell className="text-paragraph-sm-medium text-foreground">
                  <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    {branch.name}
                    {/* A closed branch sends nothing, so its missing mailbox is not news. */}
                    {branch.isActive && missesBranchEmail(branch) ? (
                      <Badge tone="warning" size="sm" dot>
                        {t('table.noEmail')}
                      </Badge>
                    ) : null}
                  </span>
                </TableCell>
                <TableCell className={branch.address ? undefined : 'text-foreground-subtle'}>
                  {branch.address ?? t('table.noAddress')}
                </TableCell>
                <TableCell kind="count">
                  {t('expiryDays', { count: branch.defaultExpiryDays })}
                </TableCell>
                <TableCell kind="status">
                  <Badge tone={branch.isActive ? 'success' : 'neutral'}>
                    {t(branch.isActive ? 'status.active' : 'status.closed')}
                  </Badge>
                </TableCell>
                <TableCell kind="actionsWide">
                  <div className="flex justify-center gap-x-1">
                    <RowActionButton
                      icon={PencilIcon}
                      label={t('edit.action')}
                      disabled={busy}
                      onClick={() => setForm({ mode: 'edit', branch })}
                    />
                    {branch.isActive ? (
                      <RowActionButton
                        icon={XCircleIcon}
                        label={t('close.action')}
                        tone="danger"
                        disabled={busy}
                        onClick={() => setClosing(branch)}
                      />
                    ) : (
                      <RowActionButton
                        icon={RotateCcwIcon}
                        label={t('reopen.action')}
                        disabled={busy}
                        onClick={() => onReopen(branch)}
                      />
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>

      <BranchFormDialog
        open={form !== null}
        onOpenChange={(open) => !open && !saving && setForm(null)}
        mode={form?.mode ?? 'create'}
        branch={form?.branch ?? null}
        onSubmit={onSubmit}
        pending={saving}
      />

      <ConfirmDialog
        open={closing !== null}
        onOpenChange={(open) => !open && !removing && setClosing(null)}
        entity={closing}
        title={t('close.title')}
        description={(branch: Branch) => t('close.description', { name: branch.name })}
        onConfirm={onClose}
        pending={removing}
        labels={{
          confirm: t('close.confirm'),
          pending: t('close.confirming'),
          cancel: t('cancel'),
        }}
      />
    </div>
  );
}
