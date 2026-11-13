'use client';

import { useMemo, useState, useTransition } from 'react';
import Link from 'next/link';
import { PencilIcon, PlusIcon, PowerIcon, PowerOffIcon, TagIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';
import { toast } from 'sonner';

import {
  Badge,
  Button,
  Combobox,
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
import { setPromotionActive } from '@/app/(protected)/settings/promotions/actions';
import {
  argentinaDay,
  PROMOTION_STATUSES,
  promotionStatus,
  type PromotionStatus,
} from '@/app/(protected)/settings/promotions/promotion-status';
import { ROUTES } from '@/config/routes';
import { useApiErrorMessage } from '@/hooks/use-api-error-message';
import { usePendingRouter } from '@/hooks/use-pending-router';
import type { Branch } from '@/lib/api/branches';
import type { Promotion } from '@/lib/api/promotions';
import { useFormatters } from '@/lib/i18n/formatters';

const ALL_STATUSES = 'ALL';

const STATUS_TONES: Record<PromotionStatus, 'success' | 'brand' | 'neutral' | 'outline'> = {
  ACTIVE: 'success',
  SCHEDULED: 'brand',
  EXPIRED: 'neutral',
  INACTIVE: 'outline',
};

interface PromotionTableProps {
  promotions: Promotion[];
  branches: Branch[];
  /* The instant the page was rendered, so the server and the browser agree on every status. */
  now: string;
}

export function PromotionTable({ promotions, branches, now }: PromotionTableProps) {
  const router = usePendingRouter();
  const fmt = useFormatters();
  const t = useTranslations('promotions');
  const message = useApiErrorMessage('promotions');
  const [statusFilter, setStatusFilter] = useState<string>(ALL_STATUSES);
  const [toggling, startToggle] = useTransition();
  const [togglingId, setTogglingId] = useState<string | null>(null);

  const statuses = useMemo(() => {
    const at = new Date(now);
    return new Map(promotions.map((promotion) => [promotion.id, promotionStatus(promotion, at)]));
  }, [promotions, now]);
  const visible = promotions.filter(
    (promotion) => statusFilter === ALL_STATUSES || statuses.get(promotion.id) === statusFilter,
  );
  // With one branch every rule reaches the same place, so the scope column would repeat itself.
  const showScope = branches.length > 1;
  const branchNames = new Map(branches.map((branch) => [branch.id, branch.name]));
  const columnCount = showScope ? 7 : 6;

  function benefitOf(promotion: Promotion): string {
    if (promotion.conditionType === 'QUANTITY_TIERED') {
      return t('benefit.tiered', { count: promotion.tiers.length });
    }
    switch (promotion.actionType) {
      case 'PERCENTAGE':
        return t('benefit.percentage', { value: fmt.value(Number(promotion.actionValue)) });
      case 'FIXED_AMOUNT':
        return t('benefit.fixedAmount', { amount: fmt.currency(promotion.actionValue) });
      case 'SPECIAL_PRICE':
        return t('benefit.specialPrice', { amount: fmt.currency(promotion.actionValue) });
    }
  }

  function validityOf(promotion: Promotion): string {
    const from = promotion.validFrom ? fmt.dateNumeric(argentinaDay(promotion.validFrom)) : null;
    const to = promotion.validTo ? fmt.dateNumeric(argentinaDay(promotion.validTo)) : null;
    if (from && to) return t('validity.range', { from, to });
    if (from) return t('validity.from', { from });
    if (to) return t('validity.to', { to });
    return t('validity.always');
  }

  function onToggle(promotion: Promotion) {
    const next = !promotion.isActive;
    setTogglingId(promotion.id);
    startToggle(async () => {
      const result = await setPromotionActive(promotion, next);
      setTogglingId(null);
      if (!result.ok) {
        toast.error(message(result.error));
        return;
      }
      toast.success(t(next ? 'toast.activated' : 'toast.deactivated', { name: promotion.name }));
    });
  }

  return (
    <div className="flex flex-col gap-y-6">
      <div className="flex flex-col gap-y-3 sm:flex-row sm:items-center sm:justify-between sm:gap-x-3">
        <Combobox
          className="w-full sm:w-56"
          options={[
            { value: ALL_STATUSES, label: t('filter.all') },
            ...PROMOTION_STATUSES.map((status) => ({
              value: status,
              label: t(`status.${status}`),
            })),
          ]}
          value={statusFilter}
          resetValue={ALL_STATUSES}
          placeholder={t('filter.placeholder')}
          onValueChange={setStatusFilter}
        />
        <Button asChild>
          <Link href={ROUTES.promotionNew}>
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
            <TableHead kind="text">{t('table.type')}</TableHead>
            <TableHead kind="text">{t('table.benefit')}</TableHead>
            {showScope ? <TableHead kind="text">{t('table.scope')}</TableHead> : null}
            <TableHead kind="text">{t('table.validity')}</TableHead>
            <TableHead kind="status">{t('table.status')}</TableHead>
            <TableHead kind="actionsWide">{t('table.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {visible.length === 0 ? (
            <TableEmptyRow
              colSpan={columnCount}
              icon={TagIcon}
              title={t(promotions.length === 0 ? 'empty.title' : 'empty.filteredTitle')}
              description={t(
                promotions.length === 0 ? 'empty.description' : 'empty.filteredDescription',
              )}
            />
          ) : (
            visible.map((promotion) => {
              const status = statuses.get(promotion.id) ?? 'INACTIVE';
              return (
                <TableRow key={promotion.id}>
                  <TableCell className="text-paragraph-sm-medium text-foreground">
                    <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
                      {promotion.name}
                      {promotion.isExclusive ? (
                        <Badge tone="warning" size="sm">
                          {t('table.exclusive')}
                        </Badge>
                      ) : null}
                    </span>
                  </TableCell>
                  <TableCell>{t(`conditionType.${promotion.conditionType}.label`)}</TableCell>
                  <TableCell>{benefitOf(promotion)}</TableCell>
                  {showScope ? (
                    <TableCell>
                      {promotion.branchId
                        ? (branchNames.get(promotion.branchId) ?? t('scope.unknownBranch'))
                        : t('scope.account')}
                    </TableCell>
                  ) : null}
                  <TableCell className="tabular-nums">{validityOf(promotion)}</TableCell>
                  <TableCell kind="status">
                    <Badge tone={STATUS_TONES[status]}>{t(`status.${status}`)}</Badge>
                  </TableCell>
                  <TableCell kind="actionsWide">
                    <div className="flex justify-center gap-x-1">
                      <RowActionButton
                        icon={PencilIcon}
                        label={t('edit.action')}
                        disabled={toggling}
                        onClick={() => router.push(ROUTES.promotionEdit(promotion.id))}
                      />
                      <RowActionButton
                        icon={promotion.isActive ? PowerOffIcon : PowerIcon}
                        label={t(promotion.isActive ? 'deactivate' : 'activate')}
                        tone={promotion.isActive ? 'danger' : 'default'}
                        disabled={toggling}
                        aria-busy={togglingId === promotion.id || undefined}
                        onClick={() => onToggle(promotion)}
                      />
                    </div>
                  </TableCell>
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
    </div>
  );
}
