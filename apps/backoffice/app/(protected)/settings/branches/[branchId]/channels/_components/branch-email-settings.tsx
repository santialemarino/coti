'use client';

import { useState, useTransition } from 'react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Callout,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  PendingButton,
} from '@repo/ui/components';
import { BranchFormDialog } from '@/app/(protected)/settings/branches/_components/branch-form-dialog';
import { updateBranch } from '@/app/(protected)/settings/branches/actions';
import type { BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import { errorCodeOf } from '@/lib/api/errors';
import { startGmailAuthorization } from '@/lib/api/gmail';
import { isTestMailbox } from '@/lib/utils/mailbox';

interface BranchEmailSettingsProps {
  branch: Branch;
}

export function BranchEmailSettings({ branch }: BranchEmailSettingsProps) {
  const t = useTranslations('branchChannels');
  const branchCopy = useTranslations('branches');
  const message = useApiErrorMessage('branches');
  const gmailMessage = useApiErrorMessage('branches.gmail');
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, startSave] = useTransition();
  const [connecting, startConnect] = useTransition();
  const testMailbox = branch.email ? isTestMailbox(branch.email) : false;
  const busy = saving || connecting;

  function onSubmit(values: BranchValues) {
    setError(null);
    startSave(async () => {
      try {
        const result = await updateBranch(branch.id, values);
        if (!result.ok) {
          setError(message(result.error));
          return;
        }
        toast.success(branchCopy('updated', { name: values.name }));
        setEditing(false);
      } catch {
        setError(message('UNREACHABLE'));
      }
    });
  }

  function onConnect() {
    const address = branch.email;
    if (!address) return;
    setError(null);
    startConnect(async () => {
      try {
        window.location.assign(await startGmailAuthorization(branch.id, address));
      } catch (error) {
        setError(gmailMessage(errorCodeOf(error)));
      }
    });
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
          <CardTitle>
            <h2>{t('email')}</h2>
          </CardTitle>
          <Badge tone={branch.email ? 'neutral' : 'warning'} size="sm" dot>
            {t(branch.email ? 'emailRegistered' : 'status.missing')}
          </Badge>
        </div>
        <CardDescription>{t('emailDescription')}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col items-start gap-y-4">
        <p className="text-paragraph-sm-medium text-foreground wrap-anywhere">
          {branch.email ?? t('noEmail')}
        </p>
        <p className="text-paragraph-sm text-foreground-muted">{t('gmailHint')}</p>
        {testMailbox ? <Callout tone="warning">{t('testMailbox')}</Callout> : null}
        {error && !editing ? <Callout tone="danger">{error}</Callout> : null}
        <div className="flex flex-wrap gap-x-3 gap-y-3">
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => {
              setError(null);
              setEditing(true);
            }}
          >
            {t('editEmail')}
          </Button>
          {branch.email && !testMailbox ? (
            <PendingButton
              variant="outline"
              pending={connecting}
              disabled={saving}
              pendingLabel={t('connecting')}
              onClick={onConnect}
            >
              {t('connectGmail')}
            </PendingButton>
          ) : !branch.email ? (
            <p className="text-paragraph-sm text-foreground-muted">{t('emailRequired')}</p>
          ) : null}
        </div>
      </CardContent>
      <BranchFormDialog
        open={editing}
        onOpenChange={(next) => !saving && setEditing(next)}
        mode="edit"
        branch={branch}
        onSubmit={onSubmit}
        pending={saving}
        error={error}
      />
    </Card>
  );
}
