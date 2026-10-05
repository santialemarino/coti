import { usePathname } from 'next/navigation';

import type { SettingsNavItem } from '@/app/(protected)/_components/settings-nav';
import { useRfqList } from '@/app/(protected)/rfqs/_components/rfq-list-context';
import { isSettingsPath, queueSelection, ROUTES } from '@/config/routes';

export type ColumnSection = 'queue' | 'settings';

/*
 * The section the path opens the left column on — none closes it — and the order it has open.
 * `atRoot` is the section's own landing, where a narrow screen shows the column as the page.
 */
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
  const atRoot =
    (section === 'queue' && pathname === ROUTES.home) ||
    (section === 'settings' && pathname === ROUTES.settings);
  return { section, rfqId: selection.rfqId, atRoot };
}
