import Link from 'next/link';
import { LayersIcon, TableIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button } from '@repo/ui/components';
import { ROUTES } from '@/config/routes';

const VIEWS = {
  queue: { href: ROUTES.home, Icon: LayersIcon, label: 'toQueue' },
  table: { href: ROUTES.rfqs, Icon: TableIcon, label: 'toTable' },
} as const;

/*
 * The way across to the orders' other view. It sits beside the "Pedidos" title on both sides — the
 * column's header and the table's page header — so it is always found in the same place.
 */
export function RfqViewLink({ to }: { to: keyof typeof VIEWS }) {
  const t = useTranslations('rfqs.view');
  const { href, Icon, label } = VIEWS[to];

  return (
    <Button asChild variant="outline" size="sm">
      <Link href={href}>
        <Icon aria-hidden="true" />
        {t(label)}
      </Link>
    </Button>
  );
}
