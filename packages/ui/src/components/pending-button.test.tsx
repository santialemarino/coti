import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { PendingButton } from './pending-button';

const LABEL = 'Guardar';
const PENDING_LABEL = 'Guardando…';

function button() {
  return screen.getByRole('button') as HTMLButtonElement;
}

describe('PendingButton', () => {
  it('shows its own label and stays operable when idle', () => {
    render(<PendingButton pendingLabel={PENDING_LABEL}>{LABEL}</PendingButton>);

    expect(button().textContent).toContain(LABEL);
    expect(button().disabled).toBe(false);
    expect(button().getAttribute('aria-busy')).toBe('false');
  });

  /*
   * Disabling itself is the point: every form in the product relies on this instead of a
   * hand-rolled `disabled={isSubmitting}`, so a double submit cannot get through.
   */
  it('disables itself and announces the wait when pending', () => {
    render(
      <PendingButton pending pendingLabel={PENDING_LABEL}>
        {LABEL}
      </PendingButton>,
    );

    expect(button().disabled).toBe(true);
    expect(button().getAttribute('aria-busy')).toBe('true');
    expect(button().textContent).toContain(PENDING_LABEL);
  });

  // An explicitly disabled button must not become operable just because nothing is pending.
  it('stays disabled when the caller disables it', () => {
    render(
      <PendingButton disabled pendingLabel={PENDING_LABEL}>
        {LABEL}
      </PendingButton>,
    );

    expect(button().disabled).toBe(true);
  });

  // It inherits Button's default, so dropping one into a form cannot submit it by accident.
  it('defaults to type="button"', () => {
    render(<PendingButton pendingLabel={PENDING_LABEL}>{LABEL}</PendingButton>);
    expect(button().getAttribute('type')).toBe('button');
  });

  it('forwards a submit type through to the real button', () => {
    render(
      <PendingButton type="submit" pendingLabel={PENDING_LABEL}>
        {LABEL}
      </PendingButton>,
    );
    expect(button().getAttribute('type')).toBe('submit');
  });
});
