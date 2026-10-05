'use client';

import { useMemo, useState } from 'react';
import { InboxIcon } from 'lucide-react';
import { LayoutGroup } from 'motion/react';
import { useTranslations } from 'next-intl';

import { EmptyState } from '@repo/ui/components';
import { RfqQueueGroup } from '@/app/(protected)/rfqs/_components/rfq-queue-group';
import { groupQueue } from '@/app/(protected)/rfqs/_components/rfq-queue-groups';
import type { RfqRecord, RfqStatus } from '@/lib/api/rfqs';

interface RfqSidebarListProps {
  records: RfqRecord[];
  activeRfqId: string | null;
}

/*
 * The queue rail: the open orders grouped by status, every group folded into a stack until it is
 * opened. Archived orders are left out — they are archived precisely so they stop appearing here;
 * the dashboard's status filter is where they are found again.
 *
 * The column outlives navigation, so which groups are open survives moving between orders and
 * sections; a reload folds them all again but the open order's.
 */
export function RfqSidebarList({ records, activeRfqId }: RfqSidebarListProps) {
  const t = useTranslations('rfqs');

  const groups = useMemo(() => groupQueue(records), [records]);
  const activeStatus =
    records.find((rfq) => rfq.id === activeRfqId && !rfq.archived)?.status ?? null;

  const [expanded, setExpanded] = useState<ReadonlySet<RfqStatus>>(
    () => new Set(activeStatus ? [activeStatus] : []),
  );
  /*
   * The open order's group unfolds whenever another order opens or the open one changes status —
   * from the table, the rail or a transition — so the selection is never buried in a stack.
   * Adjusted during render rather than in an effect, so the group is never painted folded first.
   */
  const [seen, setSeen] = useState({ id: activeRfqId, status: activeStatus });
  if (seen.id !== activeRfqId || seen.status !== activeStatus) {
    setSeen({ id: activeRfqId, status: activeStatus });
    if (activeStatus && !expanded.has(activeStatus)) {
      setExpanded(new Set(expanded).add(activeStatus));
    }
  }

  function toggle(status: RfqStatus) {
    setExpanded((previous) => {
      const next = new Set(previous);
      if (!next.delete(status)) next.add(status);
      return next;
    });
  }

  if (groups.length === 0) {
    return <EmptyState icon={InboxIcon} title={t('list.empty.title')} />;
  }

  return (
    <nav className="flex flex-col p-2 gap-y-4" aria-label={t('list.title')}>
      <LayoutGroup>
        {groups.map((group) => (
          <RfqQueueGroup
            key={group.status}
            group={group}
            expanded={expanded.has(group.status)}
            activeRfqId={activeRfqId}
            onToggle={() => toggle(group.status)}
          />
        ))}
      </LayoutGroup>
    </nav>
  );
}
