import * as React from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { DateRangePicker, type DateRangeValue } from './date-range-picker';

const LABELS = {
  from: 'Desde',
  to: 'Hasta',
  pick: 'Seleccioná una fecha',
  done: 'Listo',
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
  return screen.getByRole('button', { name: 'Período' });
}

function panel() {
  return screen.getByRole('dialog');
}

// Day buttons are named by their long Spanish date, with "Hoy, " or ", seleccionado" around it.
function day(label: string) {
  return within(panel()).getByRole('button', { name: new RegExp(`(^|, )${label}(,|$)`) });
}

function endTab(name: 'Desde' | 'Hasta') {
  return within(panel()).getByRole('button', { name: new RegExp(`^${name}`) });
}

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

  it('sets the start, then the end, and commits once both are picked', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('5 de octubre de 2026'));
    expect(onValueChange).not.toHaveBeenCalled();
    expect(endTab('Hasta').getAttribute('aria-pressed')).toBe('true');

    fireEvent.click(day('12 de octubre de 2026'));
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-05', to: '2026-10-12' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(trigger().textContent).toBe('05/10/2026 – 12/10/2026');
  });

  it('starts over from a day before the start instead of running backwards', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('20 de octubre de 2026'));
    fireEvent.click(day('10 de octubre de 2026'));
    expect(onValueChange).not.toHaveBeenCalled();
    fireEvent.click(day('15 de octubre de 2026'));

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-10', to: '2026-10-15' });
  });

  it('changes the end alone when that end is chosen first', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(endTab('Hasta'));
    fireEvent.click(day('28 de octubre de 2026'));

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-01', to: '2026-10-28' });
  });

  it('confirms a start changed alone with the done button', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    fireEvent.click(day('3 de octubre de 2026'));
    fireEvent.click(within(panel()).getByRole('button', { name: 'Listo' }));

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-03', to: '2026-10-25' });
  });

  it('drops an unfinished edit on Escape', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());
    fireEvent.click(day('3 de octubre de 2026'));
    fireEvent.click(day('2 de octubre de 2026'));

    fireEvent.keyDown(panel(), { key: 'Escape' });

    expect(onValueChange).not.toHaveBeenCalled();
    expect(trigger().textContent).toBe('01/10/2026 – 25/10/2026');
  });

  it('keeps the done button shut while an end that must be set is empty', () => {
    renderPicker({ from: '', to: '' });
    fireEvent.click(trigger());

    expect(within(panel()).getByRole('button', { name: 'Listo' }).hasAttribute('disabled')).toBe(
      true,
    );
  });

  it('lets an end that may stay open be cleared and committed empty', () => {
    const { onValueChange } = renderPicker(
      { from: '2026-10-01', to: '2026-10-25' },
      { openStart: 'Desde ya', openEnd: 'Sin vencimiento' },
    );
    fireEvent.click(trigger());

    fireEvent.click(within(panel()).getByRole('button', { name: 'Quitar fin' }));
    expect(endTab('Hasta').textContent).toContain('Sin vencimiento');
    fireEvent.click(within(panel()).getByRole('button', { name: 'Listo' }));

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith({ from: '2026-10-01', to: '' });
  });

  it('offers no clear on an end that must be set', () => {
    renderPicker({ from: '2026-10-01', to: '2026-10-25' });
    fireEvent.click(trigger());

    // Counted, not looked up by name: a clear button with no label would have no name to find.
    expect(endTab('Hasta').parentElement?.querySelectorAll('button')).toHaveLength(1);
  });

  it('commits a preset at once, and marks the one the range matches', () => {
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
  it('puts focus on the start, and moves through the days with the arrows', () => {
    const { onValueChange } = renderPicker({ from: '2026-10-05', to: '2026-10-12' });
    fireEvent.click(trigger());

    expect(document.activeElement).toBe(day('5 de octubre de 2026'));
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(day('6 de octubre de 2026'));
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(day('13 de octubre de 2026'));

    fireEvent.click(document.activeElement!);
    fireEvent.click(day('20 de octubre de 2026'));
    expect(onValueChange).toHaveBeenCalledWith({ from: '2026-10-13', to: '2026-10-20' });
  });
});
