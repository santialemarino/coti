'use client';

import { useState, useTransition } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { PackageIcon, PencilIcon, PlusIcon, PowerIcon, PowerOffIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
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
import { setComboActive } from '@/app/(protected)/settings/combos/actions';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import type { Branch } from '@/lib/api/branches';
import type { Combo } from '@/lib/api/combos';
import { useFormatters } from '@/lib/i18n/formatters';

interface ComboTableProps {
  combos: Combo[];
  branches: Branch[];
}

export function ComboTable({ combos, branches }: ComboTableProps) {
  const router = useRouter();
  const fmt = useFormatters();
  const t = useTranslations('combos');
  const message = useApiErrorMessage('combos');
  const [toggling, startToggle] = useTransition();
  const [togglingId, setTogglingId] = useState<string | null>(null);
  // With one branch every combo is sold in the same place, so the column would repeat itself.
  const showBranches = branches.length > 1;
  const openBranchIds = branches.filter((branch) => branch.isActive).map((branch) => branch.id);
  const branchNames = new Map(branches.map((branch) => [branch.id, branch.name]));
  const columnCount = showBranches ? 5 : 4;

  function onToggle(combo: Combo) {
    const next = !combo.isActive;
    setTogglingId(combo.id);
    startToggle(async () => {
      const result = await setComboActive(combo, next);
      setTogglingId(null);
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      toast.success(t(next ? 'toast.activated' : 'toast.deactivated', { name: combo.name }));
    });
  }

  return (
    <div className="flex flex-col gap-y-6">
      <div className="flex sm:justify-end">
        <Button asChild>
          <Link href={ROUTES.comboNew}>
            <PlusIcon aria-hidden="true" />
            {t('add')}
          </Link>
        </Button>
      </div>

      <Table>
        <TableCaption className="sr-only">{t('table.caption')}</TableCaption>
        <TableHeader>
          <TableRow>
            <TableHead kind="text">{t('table.name')}</TableHead>
            <TableHead kind="text">{t('table.products')}</TableHead>
            {showBranches ? <TableHead kind="text">{t('table.branches')}</TableHead> : null}
            <TableHead kind="status">{t('table.status')}</TableHead>
            <TableHead kind="actionsWide">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {combos.length === 0 ? (
            <TableEmptyRow
              colSpan={columnCount}
              icon={PackageIcon}
              title={t('empty.title')}
              description={t('empty.description')}
            />
          ) : (
            combos.map((combo) => (
              <TableRow key={combo.id}>
                <TableCell className="text-paragraph-sm-medium text-foreground">
                  {combo.name}
                </TableCell>
                <TableCell>{fmt.list(combo.items.map((item) => item.productName))}</TableCell>
                {showBranches ? (
                  <TableCell>
                    {openBranchIds.every((id) => combo.branchIds.includes(id))
                      ? t('table.allBranches')
                      : fmt.list(combo.branchIds.map((id) => branchNames.get(id) ?? '—'))}
                  </TableCell>
                ) : null}
                <TableCell kind="status">
                  <Badge tone={combo.isActive ? 'success' : 'outline'}>
                    {t(combo.isActive ? 'status.active' : 'status.inactive')}
                  </Badge>
                </TableCell>
                <TableCell kind="actionsWide">
                  <div className="flex justify-center gap-x-1">
                    <RowActionButton
                      icon={PencilIcon}
                      label={t('edit.action')}
                      disabled={toggling}
                      onClick={() => router.push(ROUTES.comboEdit(combo.id))}
                    />
                    <RowActionButton
                      icon={combo.isActive ? PowerOffIcon : PowerIcon}
                      label={t(combo.isActive ? 'deactivate' : 'activate')}
                      tone={combo.isActive ? 'danger' : 'default'}
                      disabled={toggling}
                      aria-busy={togglingId === combo.id || undefined}
                      onClick={() => onToggle(combo)}
                    />
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
