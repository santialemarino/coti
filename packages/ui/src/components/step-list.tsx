import * as React from 'react';

import { cn } from '../lib/utils';

interface StepListProps {
  /* One short instruction per step, in order. Numbering is positional. */
  steps: readonly string[];
  className?: string;
}

/*
 * Numbered instructions for a task the user performs outside the app and brings back — download a
 * template, fill it in, upload it. It is not `Stepper`: nothing here has a current position, because
 * nothing here is a state the backend tracks.
 *
 * The marker and the text are centred against each other rather than top-aligned. The marker is
 * 28px and a line of copy is 21px, so `items-start` leaves the label floating three and a half
 * pixels above the number it belongs to — small enough to look accidental, which is exactly what it
 * looks like.
 */
function StepList({ steps, className }: StepListProps) {
  return (
    <ol data-slot="step-list" className={cn('grid gap-3 md:grid-cols-3', className)}>
      {steps.map((step, index) => (
        <li
          key={step}
          className="flex items-center p-4 gap-x-3 bg-muted border border-border rounded-xl"
        >
          <span
            aria-hidden="true"
            className="grid size-7 shrink-0 place-items-center bg-primary rounded-full text-paragraph-sm-medium text-primary-foreground"
          >
            {index + 1}
          </span>
          <span className="text-paragraph-sm-medium text-foreground">{step}</span>
        </li>
      ))}
    </ol>
  );
}

export { StepList };
