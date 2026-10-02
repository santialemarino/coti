import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { SortableTableHead } from './sortable-table-head';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './table';

function classesOf(element: HTMLElement) {
  return element.className.split(/\s+/);
}

const RIGHT_IN_COMPARISON = 'group-data-[figures=end]/table:text-right';

function renderTable(figures?: 'start' | 'end') {
  render(
    <Table figures={figures}>
      <TableHeader>
        <TableRow>
          <TableHead kind="text">Producto</TableHead>
          <TableHead kind="money">Subtotal</TableHead>
          <TableHead kind="actions">Acciones</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell>Cemento</TableCell>
          <TableCell kind="money">$ 9.200,00</TableCell>
          <TableCell kind="actions">x</TableCell>
        </TableRow>
      </TableBody>
    </Table>,
  );
}

describe('Table', () => {
  it('sizes a figure column and pins its minimum, starting it on the left in a list', () => {
    renderTable();

    const head = screen.getByRole('columnheader', { name: 'Subtotal' });
    const cell = screen.getByRole('cell', { name: '$ 9.200,00' });
    expect(classesOf(head)).toEqual(
      expect.arrayContaining(['w-36', 'min-w-36', 'text-left', 'tabular-nums']),
    );
    expect(classesOf(cell)).toEqual(
      expect.arrayContaining(['text-left', 'tabular-nums', 'whitespace-nowrap']),
    );
    expect(screen.getByRole('table').dataset.figures).toBe('start');
  });

  it('right-aligns heading and cell of a figure column in a table that compares amounts', () => {
    renderTable('end');

    expect(screen.getByRole('table').dataset.figures).toBe('end');
    expect(classesOf(screen.getByRole('table'))).toContain('group/table');
    expect(classesOf(screen.getByRole('columnheader', { name: 'Subtotal' }))).toContain(
      RIGHT_IN_COMPARISON,
    );
    expect(classesOf(screen.getByRole('cell', { name: '$ 9.200,00' }))).toContain(
      RIGHT_IN_COMPARISON,
    );
  });

  it('never right-aligns copy, even in a table that compares amounts', () => {
    renderTable('end');

    expect(classesOf(screen.getByRole('cell', { name: 'Cemento' }))).not.toContain(
      RIGHT_IN_COMPARISON,
    );
  });

  it('leaves a text column unsized so it takes the slack', () => {
    renderTable();

    const head = screen.getByRole('columnheader', { name: 'Producto' });
    expect(classesOf(head)).toEqual(expect.arrayContaining(['text-left']));
    expect(head.className).not.toMatch(/\b(min-)?w-\d/);
    expect(screen.getByRole('cell', { name: 'Cemento' }).dataset.kind).toBe('text');
  });

  it('centres a one-control column', () => {
    renderTable();

    expect(classesOf(screen.getByRole('columnheader', { name: 'Acciones' }))).toEqual(
      expect.arrayContaining(['text-center']),
    );
    expect(classesOf(screen.getByRole('cell', { name: 'x' }))).toEqual(
      expect.arrayContaining(['text-center']),
    );
  });

  it('keeps an automatic layout, which is what lets text columns shrink before the table scrolls', () => {
    renderTable();

    expect(classesOf(screen.getByRole('table'))).not.toContain('table-fixed');
  });
});

describe('SortableTableHead', () => {
  it("aligns its trigger with a figure column's kind", () => {
    render(
      <table>
        <thead>
          <tr>
            <SortableTableHead
              label="Monto"
              column="total"
              kind="money"
              sortBy={null}
              sortOrder="asc"
              onSort={() => {}}
            />
          </tr>
        </thead>
      </table>,
    );

    expect(classesOf(screen.getByRole('columnheader'))).toEqual(
      expect.arrayContaining(['w-36', 'text-left', RIGHT_IN_COMPARISON]),
    );
    expect(classesOf(screen.getByRole('button', { name: 'Monto' }))).toContain(
      'group-data-[figures=end]/table:justify-end',
    );
  });
});
