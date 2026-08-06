import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { PasswordMeter } from '@/components/password-meter';
import { passwordChecks } from '@/lib/constants/password';
import messages from '@/translations/es.json';

const copy = messages.common.passwordMeter;

// The real catalog, so a renamed or missing key fails here rather than rendering its own name.
function renderMeter(password: string, invalid = false) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <PasswordMeter checks={passwordChecks(password)} invalid={invalid} />
    </NextIntlClientProvider>,
  );
}

describe('PasswordMeter', () => {
  it('lists every requirement, so the caller can satisfy them on the first try', () => {
    const view = renderMeter('');

    expect(view.getAllByRole('listitem')).toHaveLength(5);
    expect(view.getByText(copy.requirements.uppercase)).toBeTruthy();
    expect(view.getByText(copy.requirements.symbol)).toBeTruthy();
  });

  // Nothing typed is not weak, it is nothing — a strength label on an empty field reads as a verdict.
  it('names no strength while the field is empty', () => {
    const view = renderMeter('');

    expect(view.queryByText(copy.strength.weak)).toBeNull();
    expect(view.queryByText(copy.strength.strong)).toBeNull();
  });

  it.each([
    ['abc', copy.strength.weak],
    ['Abc1', copy.strength.moderate],
    ['Corralon-2026!', copy.strength.strong],
  ])('reads %p as %s', (password, label) => {
    expect(renderMeter(password).getByText(label)).toBeTruthy();
  });

  /*
   * The bar is what a screen reader gets: the requirement list is decorative markers plus text, so
   * the progress element has to carry the value on its own.
   */
  it('reports its progress to assistive technology', () => {
    const bar = renderMeter('Corralon-2026!').getByRole('progressbar');

    expect(bar.getAttribute('aria-valuenow')).toBe('100');
  });

  /*
   * What is still missing turns into an error only after a submit was refused. Before that it is a
   * hint, and colouring it red while someone types their first character is telling them off for
   * not having finished.
   */
  it('marks what is missing only once a submit has been refused', () => {
    const asHint = renderMeter('abc');
    expect(asHint.getByText(copy.requirements.uppercase).className).not.toContain(
      'text-danger-foreground',
    );
    asHint.unmount();

    const asError = renderMeter('abc', true);
    expect(asError.getByText(copy.requirements.uppercase).className).toContain(
      'text-danger-foreground',
    );
  });
});
