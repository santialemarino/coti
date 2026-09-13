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
   * Each state carries its own wash, hairline and label, and its own full-strength dot. What this
   * pins is that no two states share a family and that none of them falls back on the neutral
   * defaults — a pill that quietly renders `bg-muted` looks finished and says nothing.
   */
  it("gives every state its own colour family, in the app's own pill", () => {
    const families = new Set<string>();

    for (const status of STATUS_ORDER) {
      if (status === 'RECEIVED') continue; // RECEIVED renders the ingestion spinner, never a badge
      const view = renderBadge({ status });
      const chip = view.getByText(copy.status[status]).closest('span') as HTMLElement;
      const dot = chip.querySelector<HTMLElement>('[aria-hidden="true"]');

      const family = `status-${status.toLowerCase().replaceAll('_', '-')}`;
      expect(chip.className, `${status} needs its wash`).toContain(`bg-${family}-subtle`);
      expect(chip.className, `${status} needs its hairline`).toContain(`border-${family}-border`);
      expect(chip.className, `${status} needs its label colour`).toContain(
        `text-${family}-foreground`,
      );
      families.add(family);

      expect(dot, `${status} needs its dot`).toBeTruthy();
      expect(dot!.className).toContain(`bg-${family}`);

      for (const forbidden of ['bg-black', 'text-black', 'bg-muted', 'text-foreground-muted']) {
        expect(chip.className, `${status} leaks ${forbidden}`).not.toContain(forbidden);
      }
    }

    const archived = renderBadge({ status: 'SENT', archived: true });
    const archivedChip = archived.getByText(copy.status.ARCHIVED).closest('span') as HTMLElement;
    expect(archivedChip.className).toContain('status-archived');
    expect(archivedChip.className).not.toContain('status-sent');
    families.add('status-archived');

    // Every state but RECEIVED renders a badge, and archived adds one more. Derived rather than
    // written out, so a state added later has to bring its own family instead of borrowing one.
    expect(families.size, 'each state must map to a distinct family').toBe(STATUS_ORDER.length);
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
