import * as React from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { DateRangePicker, type DateRangeValue } from './date-range-picker';

const LABELS = {
  from: 'Desde',
  to: 'Hasta',
  pick: 'Seleccioná una fecha',
  apply: 'Aplicar',
  cancel: 'Cancelar',
  clearFrom: 'Quitar inicio',
  clearTo: 'Quitar fin',
  presets: 'Períodos',
};

type Props = Partial<React.ComponentProps<typeof DateRangePicker>>;

// Controlled, the way every form drives it; `onValueChange` sees each commit.
function Harness({ initial, ...props }: Props & { initial: DateRangeValue }) {
  const [value, setValue] = React.useState(initial);
  return (
    <DateRangePicker
      aria-label="Período"
      placeholder="Elegí un período"
      labels={LABELS}
      {...props}
      value={value}
      onValueChange={(next) => {
        setValue(next);
        props.onValueChange?.(next);
      }}
    />
  );
}

function renderPicker(initial: DateRangeValue, props: Props = {}) {
  const onValueChange = vi.fn();
  const view = render(<Harness initial={initial} {...props} onValueChange={onValueChange} />);
  return { onValueChange, ...view };
}

function trigger() {
  return screen.getByRole('combobox', { name: 'Período' });
}

function panel() {
  return screen.getByRole('dialog');
}

// Day buttons are named by their long Spanish date, with "Hoy, " or ", seleccionado" around it.
function day(label: string) {
  return within(panel()).getByRole('button', { name: new RegExp(`(^|, )${label}(,|$)`) });
}

function applyButton() {
  return within(panel()).getByRole('button', { name: 'Aplicar' });
}

function applyRange() {
  fireEvent.click(applyButton());
}

function endTab(name: 'Desde' | 'Hasta', within_: HTMLElement = panel()) {
  return within(within_).getByRole('button', { name: new RegExp(`^${name}`), hidden: true });
}

/*
 * jsdom runs no animations, so Radix unmounts a closing panel at once. Naming an exit animation, as
 * a browser would report it, keeps the panel mounted through its exit the way it is on screen.
 */
function holdClosingPanel() {
  const real = window.getComputedStyle;
  vi.spyOn(window, 'getComputedStyle').mockImplementation((element, pseudo) => {
    const style = real(element, pseudo);
    if (element.getAttribute('role') !== 'dialog') return style;
    return new Proxy(style, {
      get(target, key) {
        if (key === 'animationName') {
          return element.getAttribute('data-state') === 'open' ? 'enter' : 'exit';
        }
        const property = Reflect.get(target, key, target);
        return typeof property === 'function' ? property.bind(target) : property;
      },
    });
  });
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('DateRangePicker', () => {
  it('reads the range as Argentine dates, and the placeholder while it is empty', () => {
    const { unmount } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    expect(trigger().textContent).toBe('01/10/2026 – 25/10/2026');
    unmount();

    renderPicker({ from: '', to: '' });
    expect(trigger().textContent).toBe('Elegí un período');
  });

  it('names an open end with the wording it was given', () => {
    renderPicker(
      { from: '2026-10-01', to: '' },
      { openStart: 'Desde ya', openEnd: 'Sin vencimiento' },
    );

    expect(trigger().textContent).toBe('01/10/2026 – Sin vencimiento');
  });

  it('starts weeks on Monday and speaks Spanish', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    // The headers are hidden from assistive tech: each day's button already says its weekday.
    const headers = panel().querySelectorAll('th');
    expect(headers[0]?.getAttribute('aria-label')).toBe('lunes');
    expect(within(panel()).getByRole('grid').getAttribute('aria-label')).toBe('octubre 2026');
  });

  it('makes a range of two picks, and commits it only on Aplicar', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('5 de octubre de 2026'));
    fireEvent.click(day('12 de octubre de 2026'));
    expect(onValueChange).not.toHaveBeenCalled();
    expect(endTab('Desde').textContent).toContain('05/10/2026');
    expect(endTab('Hasta').textContent).toContain('12/10/2026');
    expect(trigger().textContent).toBe('01/10/2026 – 25/10/2026');

    applyRange();
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-05', to: '2026-10-12' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(trigger().textContent).toBe('05/10/2026 – 12/10/2026');
  });

  it('starts a new range on a pick after a complete pair', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('5 de octubre de 2026'));
    fireEvent.click(day('12 de octubre de 2026'));
    fireEvent.click(day('20 de octubre de 2026'));
    expect(endTab('Desde').textContent).toContain('20/10/2026');
    fireEvent.click(day('22 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-20', to: '2026-10-22' });
  });

  it('takes the two picks in either order, the earlier one as the start', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('20 de octubre de 2026'));
    fireEvent.click(day('8 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-08', to: '2026-10-20' });
  });

  it('makes a one-day range of the same day picked twice', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('9 de octubre de 2026'));
    fireEvent.click(day('9 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-09', to: '2026-10-09' });

    // One day is no stretch: its cell draws neither half of the band.
    fireEvent.click(trigger());
    expect(day('9 de octubre de 2026').closest('td')?.className).not.toContain('bg-linear');
  });

  it('opens with neither end chosen, so the first pick is not tied to the start', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    expect(endTab('Desde').getAttribute('aria-pressed')).toBe('false');
    expect(endTab('Hasta').getAttribute('aria-pressed')).toBe('false');
  });

  it('changes the end alone when that end is chosen first', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(endTab('Hasta'));
    fireEvent.click(day('28 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-01', to: '2026-10-28' });
  });

  it('swaps when the chosen end is moved past the other one', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-10', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(endTab('Hasta'));
    fireEvent.click(day('4 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-04', to: '2026-10-10' });
  });

  it('applies a range left open at the end', () => {
    const { onValueChange } = renderPicker(
      { from: '2026-10-01', to: '2026-10-25' },
      { openStart: 'Desde ya', openEnd: 'Sin vencimiento' },
    );
    fireEvent.click(trigger());

    fireEvent.click(day('3 de octubre de 2026'));
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-03', to: '' });
  });

  it('previews the range from the first pick to the pointer, backwards too', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('15 de octubre de 2026'));
    fireEvent.mouseEnter(day('11 de octubre de 2026'));

    const previewed = [...panel().querySelectorAll('button[data-preview]')].map(
      (b) => b.textContent,
    );
    expect(previewed).toEqual(['12', '13', '14']);
    // The ends follow the preview, so the earlier day reads as the start whichever came first.
    expect(endTab('Desde').textContent).toContain('11/10/2026');
    expect(endTab('Hasta').textContent).toContain('15/10/2026');
    // A previewed band is the lighter tint of a committed one.
    expect(day('11 de octubre de 2026').closest('td')?.className).toContain('to-accent/60');
    expect(panel().querySelectorAll('button[data-in-range]')).toHaveLength(0);
  });

  it('leaves a day past the bounds unpickable, and swaps within them', () => {
    const { onValueChange } = renderPicker(
      { from: '2026-10-05', to: '2026-10-12' },
      { min: '2026-10-03', max: '2026-10-20' },
    );
    fireEvent.click(trigger());

    fireEvent.click(day('2 de octubre de 2026'));
    fireEvent.click(day('21 de octubre de 2026'));
    expect(endTab('Desde').textContent).toContain('05/10/2026');

    fireEvent.click(day('20 de octubre de 2026'));
    fireEvent.click(day('3 de octubre de 2026'));
    applyRange();
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-03', to: '2026-10-20' });
  });

  // Aplicar commits the draft, so a preview must not outlive the focus that drew it.
  it('drops a keyboard preview once focus leaves the grid', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-05', to: '2026-10-12' });
    fireEvent.click(trigger());

    fireEvent.click(endTab('Hasta'));
    fireEvent.focus(day('20 de octubre de 2026'));
    expect(endTab('Hasta').textContent).toContain('20/10/2026');

    fireEvent.blur(day('20 de octubre de 2026'), { relatedTarget: applyButton() });
    expect(endTab('Hasta').textContent).toContain('12/10/2026');
    // Nothing was picked, so there is nothing to apply.
    expect(applyButton().hasAttribute('disabled')).toBe(true);
    expect(onValueChange).not.toHaveBeenCalled();
  });

  // A button that squares off as it leaves the range snaps its radius mid-transition, so the band is the cell's.
  it('draws the band on the cells and never reshapes a day button', () => {
    renderPicker({ from: '2026-10-05', to: '2026-10-12' });
    fireEvent.click(trigger());
    const cell = (label: string) => day(label).closest('td')!;
    const buttonsKeepTheirShape = () =>
      [...panel().querySelectorAll('td button')].every(
        // No state may reshape the button (a variant-prefixed radius) or paint the band on it.
        (button) => !/:rounded-|rounded-none|bg-accent/.test(button.className),
      );

    expect(cell('8 de octubre de 2026').className).toMatch(/(^|\s)bg-accent(\s|$)/);
    expect(cell('5 de octubre de 2026').className).toContain('bg-linear-to-r');
    expect(cell('12 de octubre de 2026').className).toContain('bg-linear-to-l');
    expect(buttonsKeepTheirShape()).toBe(true);

    fireEvent.click(day('20 de octubre de 2026'));
    fireEvent.mouseEnter(day('16 de octubre de 2026'));
    expect(cell('18 de octubre de 2026').className).toContain('bg-accent/60');
    expect(cell('16 de octubre de 2026').className).toContain('bg-linear-to-r');
    expect(buttonsKeepTheirShape()).toBe(true);

    fireEvent.mouseEnter(day('17 de octubre de 2026'));
    expect(cell('16 de octubre de 2026').className).not.toContain('bg-');
    expect(buttonsKeepTheirShape()).toBe(true);
  });

  // A closing panel stays on screen for its exit; it must show the range just applied.
  it('fades out showing the range it applied', () => {
    holdClosingPanel();
    renderPicker({ from: '2026-10-05', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('8 de octubre de 2026'));
    fireEvent.click(day('12 de octubre de 2026'));
    applyRange();

    const closing = screen.getByRole('dialog', { hidden: true });
    expect(closing.getAttribute('data-state')).toBe('closed');
    expect(endTab('Hasta', closing).textContent).toContain('12/10/2026');
    expect(closing.querySelectorAll('button[data-in-range]')).toHaveLength(3);
  });

  it('fades out showing the range it kept when the edit is cancelled', () => {
    holdClosingPanel();
    renderPicker({ from: '2026-10-05', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('8 de octubre de 2026'));
    fireEvent.click(day('12 de octubre de 2026'));
    fireEvent.click(within(panel()).getByRole('button', { name: 'Cancelar' }));

    const closing = screen.getByRole('dialog', { hidden: true });
    expect(closing.getAttribute('data-state')).toBe('closed');
    expect(endTab('Desde', closing).textContent).toContain('05/10/2026');
    expect(endTab('Hasta', closing).textContent).toContain('25/10/2026');
  });

  it('drops the draft on Cancelar and returns focus to the field', async () => {
    const { onValueChange } = renderPicker({ from: '2026-10-05', to: '2026-10-25' });
    // A browser focuses the field it was clicked on; jsdom does not.
    trigger().focus();
    fireEvent.click(trigger());
    fireEvent.click(day('8 de octubre de 2026'));
    fireEvent.click(day('12 de octubre de 2026'));

    fireEvent.click(within(panel()).getByRole('button', { name: 'Cancelar' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    // Radix hands focus back once the panel has unmounted.
    await waitFor(() => expect(document.activeElement).toBe(trigger()));

    fireEvent.click(trigger());
    expect(endTab('Desde').textContent).toContain('05/10/2026');
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it('drops an unfinished edit on Escape', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());
    fireEvent.click(day('3 de octubre de 2026'));

    fireEvent.keyDown(panel(), { key: 'Escape' });

    expect(onValueChange).not.toHaveBeenCalled();
    expect(trigger().textContent).toBe('01/10/2026 – 25/10/2026');

    // Reopened, the panel shows the range as it was, not the dropped pick.
    fireEvent.click(trigger());
    expect(endTab('Desde').textContent).toContain('01/10/2026');
    expect(endTab('Hasta').textContent).toContain('25/10/2026');
    // …and forgets the dropped first pick, so one pick does not complete a range.
    fireEvent.click(day('7 de octubre de 2026'));
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it('keeps Aplicar shut while an end that must be set is empty, or nothing changed', () => {
    renderPicker({ from: '2026-10-05', to: '2026-10-25' });
    fireEvent.click(trigger());
    expect(applyButton().hasAttribute('disabled')).toBe(true);

    fireEvent.click(day('8 de octubre de 2026'));
    expect(applyButton().hasAttribute('disabled')).toBe(true);

    fireEvent.click(day('12 de octubre de 2026'));
    expect(applyButton().hasAttribute('disabled')).toBe(false);
  });

  it('lets an end that may stay open be cleared and committed empty', () => {
    const { onValueChange } = renderPicker(
      { from: '2026-10-01', to: '2026-10-25' },
      { openStart: 'Desde ya', openEnd: 'Sin vencimiento' },
    );
    fireEvent.click(trigger());

    fireEvent.click(within(panel()).getByRole('button', { name: 'Quitar fin' }));
    expect(endTab('Hasta').textContent).toContain('Sin vencimiento');
    expect(within(panel()).queryByRole('button', { name: 'Quitar fin' })).toBeNull();
    applyRange();

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-01', to: '' });
  });

  // Named by its label like any field, it still has to say which dates it holds.
  it('exposes the chosen range as the value of a combobox that opens a dialog', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });

    expect(trigger().getAttribute('aria-haspopup')).toBe('dialog');
    expect(trigger().textContent).toBe('01/10/2026 – 25/10/2026');
  });

  it('waits for the other pick when the end is chosen while the start is empty, then swaps', () => {
    // An empty range opens on the current month.
    vi.useFakeTimers({ now: new Date(2026, 10, 13), toFake: ['Date'] });
    const { onValueChange } = renderPicker({ from: '', to: '' });
    fireEvent.click(trigger());

    fireEvent.click(endTab('Hasta'));
    fireEvent.click(day('10 de noviembre de 2026'));
    expect(onValueChange).not.toHaveBeenCalled();

    fireEvent.click(day('20 de noviembre de 2026'));
    applyRange();
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-11-10', to: '2026-11-20' });
  });

  it('offers no clear on an end that must be set', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    // Counted, not looked up by name: a clear button with no label would have no name to find.
    expect(endTab('Hasta').parentElement?.querySelectorAll('button')).toHaveLength(1);
  });

  it('fills the draft from a preset without committing, and marks the one it matches', () => {
    const presets = [
      { label: 'Octubre', value: { from: '2026-10-01', to: '2026-10-31' } },
      { label: 'Septiembre', value: { from: '2026-09-01', to: '2026-09-30' } },
    ];
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-31' }, { presets });
    fireEvent.click(trigger());

    expect(
      within(panel()).getByRole('button', { name: 'Octubre' }).getAttribute('aria-pressed'),
    ).toBe('true');
    fireEvent.click(within(panel()).getByRole('button', { name: 'Septiembre' }));
    expect(onValueChange).not.toHaveBeenCalled();
    expect(
      within(panel()).getByRole('button', { name: 'Septiembre' }).getAttribute('aria-pressed'),
    ).toBe('true');
    expect(endTab('Desde').textContent).toContain('01/09/2026');

    applyRange();
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith(presets[1]!.value);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('marks both ends selected and greys out days past the bounds', () => {
    renderPicker({ from: '2026-10-05', to: '2026-10-12' }, { max: '2026-10-20' });
    fireEvent.click(trigger());

    expect(day('5 de octubre de 2026').closest('td')?.getAttribute('aria-selected')).toBe('true');
    expect(day('12 de octubre de 2026').closest('td')?.getAttribute('aria-selected')).toBe('true');
    expect(day('21 de octubre de 2026').hasAttribute('disabled')).toBe(true);
  });

  it('submits each end under its own name in a native form', () => {
    const { container } = renderPicker(
      { from: '2026-10-01', to: '2026-10-25' },
      { names: { from: 'from', to: 'to' } },
    );

    const form = document.createElement('form');
    container
      .querySelectorAll('input[type="hidden"]')
      .forEach((input) => form.append(input.cloneNode()));
    expect([...new FormData(form)]).toEqual([
      ['from', '2026-10-01'],
      ['to', '2026-10-25'],
    ]);
  });

  // Day-picker owns the grid's keyboard; the panel has to hand it focus for the arrows to work.
  it('puts focus on the start, and picks with the keyboard in either order', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-05', to: '2026-10-12' });
    fireEvent.click(trigger());

    expect(document.activeElement).toBe(day('5 de octubre de 2026'));
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(day('6 de octubre de 2026'));
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(day('13 de octubre de 2026'));

    // A pressed day (Enter or Space) is a click, so the keyboard picks backwards like the pointer.
    fireEvent.click(document.activeElement!);
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowUp' });
    expect(document.activeElement).toBe(day('6 de octubre de 2026'));
    fireEvent.click(document.activeElement!);
    expect(onValueChange).not.toHaveBeenCalled();
    applyRange();
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-06', to: '2026-10-13' });
  });
});
