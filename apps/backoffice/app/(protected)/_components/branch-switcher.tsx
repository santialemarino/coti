'use client';

import { useTransition } from 'react';
import { Building2Icon, StoreIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Combobox, type ComboboxOption } from '@repo/ui/components';
import { FitLabel } from '@/app/(protected)/_components/fit-label';
import { selectBranch } from '@/app/(protected)/actions';
import type { Branch } from '@/lib/api/branches';
import { ALL_BRANCHES } from '@/lib/constants/branch';

// From here a list stops being scannable and the search box earns the click it costs.
const SEARCHABLE_FROM = 8;
// The 36px trigger sits 14px above the 64px header's border; the list opens 6px past it.
const BRANCH_MENU_OFFSET = 20;

interface BranchSwitcherProps {
  branches: Branch[];
  activeBranchId: string | null;
  isAdmin: boolean;
}

export function BranchSwitcher({ branches, activeBranchId, isAdmin }: BranchSwitcherProps) {
  const t = useTranslations('common.branch');
  const [pending, startTransition] = useTransition();

  // "Todas" is account-wide, an admin's reach alone; a seller never sees an option the API
  // reads as something wider than their assignments.
  const options = [
    ...(isAdmin
      ? [{ value: ALL_BRANCHES, label: t('all'), icon: <Building2Icon aria-hidden="true" /> }]
      : []),
    ...branches.map((branch) => ({
      value: branch.id,
      label: branch.name,
      icon: <StoreIcon aria-hidden="true" />,
    })),
  ];

  /*
   * One branch is one outcome: a seller has nowhere to switch to, and an admin's "todas" reaches the
   * same single branch. A choice with one outcome is not a choice, so it is shown, not offered.
   */
  const sole = branches.length === 1 ? branches[0] : undefined;

  // A narrow trigger cuts "Todas las sucursales" mid-word; the short form says the same thing whole.
  function triggerLabel(option: ComboboxOption) {
    return option.value === ALL_BRANCHES ? (
      <FitLabel full={t('all')} short={t('allShort')} />
    ) : (
      option.label
    );
  }

  function onValueChange(value: string) {
    startTransition(async () => {
      await selectBranch(value);
    });
  }

  if (sole) {
    return (
      <p className="flex shrink h-9 w-44 min-w-0 sm:w-56 items-center px-3 gap-x-2 text-paragraph-sm text-foreground">
        <StoreIcon aria-hidden="true" className="size-4 shrink-0 text-foreground-muted" />
        <span className="sr-only">{t('label')}: </span>
        <span className="truncate">{sole.name}</span>
      </p>
    );
  }

  return (
    <Combobox
      options={options}
      value={activeBranchId ?? ALL_BRANCHES}
      onValueChange={onValueChange}
      placeholder={t('placeholder')}
      searchable={branches.length >= SEARCHABLE_FROM}
      searchPlaceholder={t('search')}
      emptyLabel={t('empty')}
      disabled={pending}
      aria-label={t('label')}
      triggerLabel={triggerLabel}
      sideOffset={BRANCH_MENU_OFFSET}
      className="w-44 min-w-0 shrink sm:w-56"
    />
  );
}
