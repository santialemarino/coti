import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { SortableTableHead } from './sortable-table-head';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from './table';

function classesOf(element: HTMLElement) {
  return element.className.split(/\s+/);
}

function renderTable() {
  render(
    <Table>
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
  it('sizes a figure column and pins its minimum, heading and cell right-aligned', () => {
    renderTable();

    const head = screen.getByRole('columnheader', { name: 'Subtotal' });
    const cell = screen.getByRole('cell', { name: '$ 9.200,00' });
    expect(classesOf(head)).toEqual(
      expect.arrayContaining(['w-36', 'min-w-36', 'text-right', 'tabular-nums']),
    );
    expect(classesOf(cell)).toEqual(
      expect.arrayContaining(['text-right', 'tabular-nums', 'whitespace-nowrap']),
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
      expect.arrayContaining(['w-36', 'text-right']),
    );
    expect(classesOf(screen.getByRole('button', { name: 'Monto' }))).toEqual(
      expect.arrayContaining(['justify-end']),
    );
  });
});
