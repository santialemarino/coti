import * as React from 'react';
import { CheckIcon } from 'lucide-react';

import { cn } from '../lib/utils';

export interface StepperStep {
  id: string;
  label: string;
  /* Shown under the marker — the date a step was reached, a count, a short note. */
  meta?: string;
}

interface StepperProps {
  steps: StepperStep[];
  /* The step in progress. Everything before it reads as done, everything after as pending. */
  currentIndex: number;
  className?: string;
}

/*
 * The lifecycle rail: where a thing is in a fixed sequence of states. Connectors are real flex
 * children rather than pseudo-elements, so each segment can be coloured by whether it has been
 * passed, and a step count change needs no CSS.
 *
 * An ordered list with `aria-current="step"`, so the sequence and the position in it are conveyed
 * without relying on colour.
 */
function Stepper({ steps, currentIndex, className }: StepperProps) {
  const hasMeta = steps.some((step) => step.meta);

  return (
    <ol data-slot="stepper" className={cn('flex w-full items-start', className)}>
      {steps.map((step, index) => {
        const isDone = index < currentIndex;
        const isCurrent = index === currentIndex;

        return (
          <li
            key={step.id}
            aria-current={isCurrent ? 'step' : undefined}
            className="flex flex-1 flex-col items-center gap-y-2"
          >
            <span
              className={cn(
                'px-1 text-center text-paragraph-mini-medium sm:text-paragraph-xs-medium',
                isDone || isCurrent ? 'text-foreground' : 'text-foreground-subtle',
              )}
            >
              {step.label}
            </span>

            <div className="flex w-full items-center" aria-hidden="true">
              <span
                className={cn(
                  'h-0.5 flex-1 rounded-full transition-colors duration-300 ease-out-soft',
                  index === 0 && 'invisible',
                  index <= currentIndex ? 'bg-primary' : 'bg-border',
                )}
              />
              <span
                className={cn(
                  'grid size-6 shrink-0 place-items-center border-2 rounded-full',
                  'transition-[background-color,border-color,box-shadow] duration-300 ease-out-soft',
                  isDone && 'bg-primary border-primary text-primary-foreground',
                  isCurrent && 'bg-background border-primary shadow-e1 ring-3 ring-ring/25',
                  !isDone && !isCurrent && 'bg-background border-border',
                )}
              >
                {isDone ? <CheckIcon className="size-3.5" /> : null}
                {isCurrent ? <span className="size-2 bg-primary rounded-full" /> : null}
              </span>
              <span
                className={cn(
                  'h-0.5 flex-1 rounded-full transition-colors duration-300 ease-out-soft',
                  index === steps.length - 1 && 'invisible',
                  index < currentIndex ? 'bg-primary' : 'bg-border',
                )}
              />
            </div>

            {hasMeta ? (
              <span className="min-h-4 px-1 text-center text-paragraph-mini text-foreground-subtle">
                {step.meta}
              </span>
            ) : null}
          </li>
        );
      })}
    </ol>
  );
}

export { Stepper };
