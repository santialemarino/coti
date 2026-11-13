import * as React from 'react';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { DatePicker } from './date-picker';

function Harness({
  initial,
  onValueChange,
}: {
  initial: string;
  onValueChange: (value: string) => void;
}) {
  const [value, setValue] = React.useState(initial);
  return (
    <DatePicker
      aria-label="Inicio de actividades"
      placeholder="Seleccioná una fecha"
      value={value}
      onValueChange={(next) => {
        setValue(next);
        onValueChange(next);
      }}
    />
  );
}

function renderPicker(initial: string) {
  const onValueChange = vi.fn();
  render(<Harness initial={initial} onValueChange={onValueChange} />);
  return onValueChange;
}

function trigger() {
  return screen.getByRole('combobox', { name: 'Inicio de actividades' });
}

function day(label: string) {
  return within(screen.getByRole('dialog')).getByRole('button', {
    name: new RegExp(`(^|, )${label}(,|$)`),
  });
}

describe('DatePicker', () => {
  it('reads the day as an Argentine date, and the placeholder while none is set', () => {
    renderPicker('2019-03-04');
    expect(trigger().textContent).toBe('04/03/2019');
  });

  it('shows the placeholder while no day is set', () => {
    renderPicker('');
    expect(trigger().textContent).toBe('Seleccioná una fecha');
  });

  // Left open, the calendar covered the next field and swallowed the click meant for it.
  it('closes once a day is picked, and hands back the calendar day', () => {
    const onValueChange = renderPicker('2019-03-04');
    fireEvent.click(trigger());

    fireEvent.click(day('15 de marzo de 2019'));

    expect(onValueChange).toHaveBeenCalledExactlyOnceWith('2019-03-15');
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(trigger().textContent).toBe('15/03/2019');
  });

  it('keeps the day and closes when the chosen day is pressed again', () => {
    const onValueChange = renderPicker('2019-03-04');
    fireEvent.click(trigger());

    fireEvent.click(day('4 de marzo de 2019'));

    expect(onValueChange).not.toHaveBeenCalled();
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(trigger().textContent).toBe('04/03/2019');
  });

  it('opens on the chosen month with the day focused, so the arrows work at once', () => {
    renderPicker('2019-03-04');
    fireEvent.click(trigger());

    expect(document.activeElement).toBe(day('4 de marzo de 2019'));
  });

  // Years back by the design system's dropdown, not the browser's select.
  it('offers the month and year as the design system dropdown', () => {
    renderPicker('2019-03-04');
    fireEvent.click(trigger());

    const dialog = screen.getByRole('dialog');
    expect(within(dialog).queryByRole('combobox', { name: 'Elegir el año' })).not.toBeNull();
    expect(dialog.querySelector('select')).toBeNull();
  });
});
