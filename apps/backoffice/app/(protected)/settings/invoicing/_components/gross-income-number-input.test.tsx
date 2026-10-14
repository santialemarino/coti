import { createRef, useState } from 'react';
import { createEvent, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { beforeInput, browserEdit, typeText } from '@repo/vitest-config/input-edits';
import { GrossIncomeNumberInput } from '@/app/(protected)/settings/invoicing/_components/gross-income-number-input';

function Harness({ onKeyDown }: { onKeyDown?: () => void }) {
  const [value, setValue] = useState('');
  return (
    <>
      <GrossIncomeNumberInput
        aria-label="IIBB"
        value={value}
        onChange={setValue}
        onKeyDown={onKeyDown}
      />
      <button type="button" onClick={() => setValue('20123456789')}>
        same as CUIT
      </button>
    </>
  );
}

function input() {
  return screen.getByRole('textbox', { name: 'IIBB' }) as HTMLInputElement;
}

function undo() {
  const event = createEvent.keyDown(input(), { key: 'z', metaKey: true });
  fireEvent(input(), event);
  return event;
}

function start() {
  input().focus();
  input().setSelectionRange(0, 0);
}

describe('GrossIncomeNumberInput', () => {
  it('keeps digits and hyphens, dropping anything else', () => {
    render(<Harness />);
    start();

    typeText(input(), '901-23a4');

    expect(input().value).toBe('901-234');
  });

  it('undoes a filtered paste on its own, with the typing before it whole', () => {
    render(<Harness />);
    start();
    typeText(input(), '901');
    browserEdit(input(), '901 234.567-8', { inputType: 'insertFromPaste' });
    expect(input().value).toBe('901234567-8');

    expect(undo().defaultPrevented).toBe(true);
    expect(input().value).toBe('901');
    expect([input().selectionStart, input().selectionEnd]).toEqual([3, 3]);

    undo();
    expect(input().value).toBe('');
    expect(beforeInput(input(), 'historyRedo')).toBe(true);
    expect(input().value).toBe('901');
  });

  it('takes its own step when the browser undoes without announcing it', () => {
    render(<Harness />);
    start();
    typeText(input(), '901');
    browserEdit(input(), '901-234', { inputType: 'insertFromPaste' });

    browserEdit(input(), '901-23', { inputType: 'historyUndo', announced: false });

    expect(input().value).toBe('901');
  });

  it('starts over when the value is filled from outside', () => {
    render(<Harness />);
    start();
    typeText(input(), '9');
    fireEvent.click(screen.getByRole('button', { name: 'same as CUIT' }));

    undo();

    expect(input().value).toBe('20123456789');
  });

  it('still hands its keys to the caller and its ref to the form', () => {
    const onKeyDown = vi.fn();
    const ref = createRef<HTMLInputElement>();
    render(
      <>
        <Harness onKeyDown={onKeyDown} />
        <GrossIncomeNumberInput aria-label="other" ref={ref} onChange={vi.fn()} />
      </>,
    );

    fireEvent.keyDown(input(), { key: 'Enter' });

    expect(onKeyDown).toHaveBeenCalledOnce();
    expect(ref.current).toBe(screen.getByRole('textbox', { name: 'other' }));
  });
});
