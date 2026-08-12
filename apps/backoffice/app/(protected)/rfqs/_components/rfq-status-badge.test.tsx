import { render } from '@testing-library/react';
import { NextIntlClientProvider } from 'next-intl';
import { describe, expect, it } from 'vitest';

import {
  hasQuoteTotal,
  RfqStatusBadge,
  STATUS_COLOUR,
  STATUS_ORDER,
  type RfqStatusBadgeProps,
} from '@/app/(protected)/rfqs/_components/rfq-status-badge';
import messages from '@/translations/es.json';

const copy = messages.rfqs;

function renderBadge(props: RfqStatusBadgeProps) {
  return render(
    <NextIntlClientProvider
      locale="es"
      messages={messages}
      timeZone="America/Argentina/Buenos_Aires"
    >
      <RfqStatusBadge {...props} />
    </NextIntlClientProvider>,
  );
}

describe('RfqStatusBadge', () => {
  it('shows the ingestion spinner while RECEIVED, never the badge', () => {
    const view = renderBadge({ status: 'RECEIVED' });

    expect(view.getByRole('status')).toBeTruthy();
    expect(view.getByText(copy.processing.ingestion)).toBeTruthy();
    expect(view.queryByText(copy.status.RECEIVED)).toBeNull();
  });

  it('shows the generation spinner while the AI is producing the quote', () => {
    const view = renderBadge({ status: 'GENERATED', processing: true });

    expect(view.getByRole('status')).toBeTruthy();
    expect(view.getByText(copy.processing.quote)).toBeTruthy();
    expect(view.queryByText(copy.status.GENERATED)).toBeNull();
  });

  it('renders the static badge for a settled status', () => {
    const view = renderBadge({ status: 'QUOTED' });

    expect(view.getByText(copy.status.QUOTED)).toBeTruthy();
    expect(view.queryByRole('status')).toBeNull();
  });

  it('shows the archived badge over the real status when the flag is set', () => {
    const view = renderBadge({ status: 'SENT', archived: true });

    expect(view.getByText(copy.status.ARCHIVED)).toBeTruthy();
    expect(view.queryByText(copy.status.SENT)).toBeNull();
  });

  it('maps every status in the domain to a colour', () => {
    for (const status of STATUS_ORDER) {
      expect(STATUS_COLOUR[status], `${status} needs a colour`).not.toBeUndefined();
    }
  });

  /*
   * The Figma spec: the label is painted with the state colour and the backdrop tints that same
   * colour at 20% opacity — never a solid fill, never default black text. If the tinted backdrop is
   * ever replaced by a solid `bg-status-*` or the neutral defaults sneak back in, this test fails.
   */
  it('gives every state its own colour as a 20% tinted chip, never a solid pill', () => {
    const colours = new Set<string>();

    for (const status of STATUS_ORDER) {
      if (status === 'RECEIVED') continue; // RECEIVED renders the ingestion spinner, never a badge
      const view = renderBadge({ status });
      const label = view.getByText(copy.status[status]) as HTMLElement;
      const chip = label.parentElement as HTMLElement;
      const backdrop = chip.querySelector<HTMLElement>('[aria-hidden="true"]');

      const colour = `text-status-${status.toLowerCase().replace('_', '-')}`;
      expect(chip.className, `${status} must be painted with its colour`).toContain(colour);
      expect(chip.className, `${status} must not use a solid status fill`).not.toContain(
        'bg-status',
      );
      colours.add(colour);

      expect(backdrop, `${status} needs the tinted backdrop`).toBeTruthy();
      expect(backdrop!.className).toContain('bg-current');
      expect(backdrop!.className).toContain('opacity-20');
      expect(backdrop!.className).not.toContain('bg-status');

      expect(label.className, `${status} label should inherit the colour`).not.toContain(
        'text-foreground',
      );

      for (const forbidden of [
        'bg-black',
        'text-black',
        'bg-muted',
        'border-border',
        'rounded-full',
      ]) {
        expect(chip.className, `${status} leaks ${forbidden}`).not.toContain(forbidden);
      }
    }

    const archived = renderBadge({ status: 'SENT', archived: true });
    const archivedLabel = archived.getByText(copy.status.ARCHIVED) as HTMLElement;
    const archivedChip = archivedLabel.parentElement as HTMLElement;
    expect(archivedChip.className).toContain('text-status-archived');
    expect(archivedChip.className).not.toContain('text-status-sent');
    colours.add('text-status-archived');

    expect(colours.size, 'each state must map to a distinct colour').toBe(7);
  });

  it('recognises which statuses carry a definitive quote total', () => {
    for (const status of ['QUOTED', 'SENT', 'ACCEPTED', 'REJECTED']) {
      expect(hasQuoteTotal(status as keyof typeof STATUS_COLOUR)).toBe(true);
    }
    for (const status of ['RECEIVED', 'GENERATED', 'CHANGE_REQUESTED']) {
      expect(hasQuoteTotal(status as keyof typeof STATUS_COLOUR)).toBe(false);
    }
  });
});
