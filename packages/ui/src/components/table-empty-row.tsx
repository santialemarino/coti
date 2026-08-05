import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { EmptyState } from './empty-state';
import { TableCell, TableRow } from './table';

interface TableEmptyRowProps {
  colSpan: number;
  icon: LucideIcon;
  title: string;
  description?: string;
}

/*
 * One component for a table with no rows, so the empty cell's padding and alignment can't drift
 * between tables. `p-0` hands spacing to EmptyState, which owns it.
 */
function TableEmptyRow({ colSpan, icon, title, description }: TableEmptyRowProps) {
  return (
    <TableRow className="hover:bg-transparent">
      <TableCell colSpan={colSpan} className="p-0">
        <EmptyState icon={icon} title={title} description={description} />
      </TableCell>
    </TableRow>
  );
}

export { TableEmptyRow };
