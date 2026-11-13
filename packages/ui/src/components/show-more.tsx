'use client';

import { Button } from './button';

interface ShowMoreProps {
  visible: number;
  total: number;
  step: number;
  onShowMore: () => void;
  onShowAll: () => void;
  labels: {
    /* How much of the list is on screen, e.g. "Mostrando 25 de 300 filas". */
    status: string;
    /* The next step, e.g. "Ver 25 más"; also used, alone, when one step reveals the rest. */
    more: string;
    all: string;
  };
}

/*
 * The end of a list revealed in steps: how much is showing, the next step, and everything at once.
 * "All" is offered only while more than one step remains, so the two buttons never do the same.
 * Renders nothing once the whole list is showing.
 */
function ShowMore({ visible, total, step, onShowMore, onShowAll, labels }: ShowMoreProps) {
  if (visible >= total) return null;
  const remaining = total - visible;

  return (
    <div
      data-slot="show-more"
      className="flex flex-col items-center gap-3 sm:flex-row sm:justify-center"
    >
      <p role="status" className="text-paragraph-sm text-foreground-muted">
        {labels.status}
      </p>
      <div className="flex flex-wrap justify-center gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onShowMore}>
          {labels.more}
        </Button>
        {remaining > step ? (
          <Button type="button" variant="outline" size="sm" onClick={onShowAll}>
            {labels.all}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

export { ShowMore };
