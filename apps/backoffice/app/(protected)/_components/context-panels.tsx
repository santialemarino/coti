'use client';

import { usePathname } from 'next/navigation';
import { useTranslations } from 'next-intl';

import { cn } from '@repo/ui/lib';
import { SettingsNav, type SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { RfqSidebarList } from '@/app/(protected)/rfqs/_components/rfq-sidebar-list';
import { RfqViewLink } from '@/app/(protected)/rfqs/_components/rfq-view-link';
import { isSettingsPath, queueSelection } from '@/config/routes';

export type ColumnSection = 'queue' | 'settings';

// The section the path opens the left column on — none closes it — and the order it has open.
export function useColumnSection(settingsNav: SettingsNavItem[]) {
  const pathname = usePathname();
  const { hasQueue, loadFailed } = useRfqList();

  const selection = queueSelection(pathname);
  const section: ColumnSection | null =
    hasQueue && !loadFailed && selection.inQueue
      ? 'queue'
      : settingsNav.length > 0 && isSettingsPath(pathname)
        ? 'settings'
        : null;
  return { section, rfqId: selection.rfqId };
}

interface ContextPanelsProps {
  section: ColumnSection | null;
  activeRfqId: string | null;
  settingsNav: SettingsNavItem[];
}

/*
 * What the left column holds — the queue or the settings sections — wherever it is shown: beside the
 * page from lg up, in the menu sheet below it. Both stay mounted so the queue keeps its open stacks
 * across a visit to settings; the one on show fades in each time it returns.
 */
export function ContextPanels({ section, activeRfqId, settingsNav }: ContextPanelsProps) {
  const t = useTranslations('rfqs');
  const tSettings = useTranslations('settings');
  const { records, hasQueue } = useRfqList();

  return (
    <>
      {hasQueue ? (
        <ColumnPanel
          hidden={section !== 'queue'}
          title={t('list.title')}
          action={<RfqViewLink to="table" />}
        >
          <RfqSidebarList records={records} activeRfqId={activeRfqId} />
        </ColumnPanel>
      ) : null}
      {settingsNav.length > 0 ? (
        <ColumnPanel hidden={section !== 'settings'} title={tSettings('title')}>
          <SettingsNav title={tSettings('title')} items={settingsNav} />
        </ColumnPanel>
      ) : null}
    </>
  );
}

interface ColumnPanelProps {
  hidden: boolean;
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}

function ColumnPanel({ hidden, title, action, children }: ColumnPanelProps) {
  return (
    <div
      className={cn(
        'flex flex-col animate-in fade-in-0 duration-300 ease-in-out-soft',
        hidden && 'hidden',
      )}
    >
      {/* One height whether or not there is an action, so the title never moves between sections;
          aligned with the rows below — the title with their text, the action with their edge. */}
      <div className="flex h-14 items-center justify-between pr-3 pl-6 gap-x-2 bg-background sticky top-0 z-10">
        <h2 className="text-paragraph-xs-medium text-foreground-subtle uppercase">{title}</h2>
        {action}
      </div>
      {children}
    </div>
  );
}
