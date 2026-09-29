import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from './dialog';
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from './sheet';

/*
 * jsdom runs no animations, so Radix would unmount a closing dialog at once and there would be no
 * exit to observe. Reporting a different animation name per state is what makes Radix hold the
 * closing element, exactly as a browser does while the fade-out plays. Radix keeps the declaration
 * it read on mount, so the state is read on every access.
 */
function stubAnimations() {
  const real = window.getComputedStyle.bind(window);
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element, pseudo) => {
    const styles = real(element, pseudo);
    return new Proxy(styles, {
      get(target, property) {
        if (property === 'animationName') {
          const state = element.getAttribute('data-state');
          if (state === 'open') return 'enter';
          if (state === 'closed') return 'exit';
          return 'none';
        }
        const value = Reflect.get(target, property, target);
        return typeof value === 'function' ? value.bind(target) : value;
      },
    });
  });
}

function Harness({ open, title }: { open: boolean; title: string }) {
  return (
    <Dialog open={open}>
      <DialogContent>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>Descripción</DialogDescription>
      </DialogContent>
    </Dialog>
  );
}

describe('DialogContent', () => {
  beforeEach(stubAnimations);
  afterEach(() => vi.restoreAllMocks());

  it('follows its children while open', () => {
    const { rerender } = render(<Harness open title="Editar sucursal" />);
    rerender(<Harness open title="Crear sucursal" />);

    expect(screen.getByRole('dialog').textContent).toContain('Crear sucursal');
  });

  /*
   * The caller closes and resets in the same commit. What must be on screen during the exit is the
   * dialog the caller was looking at, not the reset one.
   */
  it('keeps showing what it showed when open while it animates out', () => {
    const { rerender } = render(<Harness open title="Editar sucursal" />);
    rerender(<Harness open={false} title="Crear sucursal" />);

    const dialog = document.querySelector('[data-slot="dialog-content"]');
    expect(dialog?.getAttribute('data-state')).toBe('closed');
    expect(dialog?.textContent).toContain('Editar sucursal');
    expect(dialog?.textContent).not.toContain('Crear sucursal');
  });
});

// With no `open` prop the root keeps its own state, and still tells the caller about each change.
describe('an uncontrolled overlay', () => {
  it('opens from its trigger and closes from its close button', () => {
    const onOpenChange = vi.fn();
    render(
      <Dialog onOpenChange={onOpenChange}>
        <DialogTrigger>Abrir</DialogTrigger>
        <DialogContent>
          <DialogTitle>Método de envío</DialogTitle>
          <DialogDescription>Descripción</DialogDescription>
        </DialogContent>
      </Dialog>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Abrir' }));
    expect(screen.getByRole('dialog').textContent).toContain('Método de envío');

    fireEvent.click(screen.getByRole('button', { name: 'Cerrar' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(onOpenChange.mock.calls).toEqual([[true], [false]]);
  });

  it('opens a sheet from its trigger', () => {
    render(
      <Sheet>
        <SheetTrigger>Filtros</SheetTrigger>
        <SheetContent aria-describedby={undefined}>
          <SheetTitle>Filtros del listado</SheetTitle>
        </SheetContent>
      </Sheet>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Filtros' }));
    expect(screen.getByRole('dialog').textContent).toContain('Filtros del listado');
  });
});
