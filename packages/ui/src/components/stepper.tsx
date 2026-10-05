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
  /* `adaptive` runs down the screen below `sm`, where a row of labelled, dated steps cannot fit. */
  orientation?: 'horizontal' | 'adaptive';
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
function Stepper({ steps, currentIndex, orientation = 'horizontal', className }: StepperProps) {
  const hasMeta = steps.some((step) => step.meta);
  const adaptive = orientation === 'adaptive';

  return (
    <ol
      data-slot="stepper"
      data-orientation={orientation}
      className={cn(
        'flex w-full items-start',
        adaptive && 'max-sm:flex-col max-sm:items-stretch',
        className,
      )}
    >
      {steps.map((step, index) => {
        const isDone = index < currentIndex;
        const isCurrent = index === currentIndex;

        return (
          <li
            key={step.id}
            aria-current={isCurrent ? 'step' : undefined}
            // Below sm an adaptive step is a row: the rail on the left, its label and date beside it.
            className={cn(
              'flex flex-1 flex-col items-center gap-y-2',
              adaptive &&
                'max-sm:grid max-sm:grid-cols-[auto_minmax(0,1fr)] max-sm:grid-rows-[auto_auto] max-sm:items-center max-sm:gap-x-3 max-sm:gap-y-0',
            )}
          >
            <span
              className={cn(
                'px-1 text-center text-paragraph-mini-medium sm:text-paragraph-xs-medium',
                adaptive &&
                  'max-sm:col-start-2 max-sm:row-start-1 max-sm:self-end max-sm:px-0 max-sm:pt-2 max-sm:text-left max-sm:text-paragraph-sm-medium',
                isDone || isCurrent ? 'text-foreground' : 'text-foreground-subtle',
              )}
            >
              {step.label}
            </span>

            <div
              className={cn(
                'flex w-full items-center',
                adaptive &&
                  'max-sm:col-start-1 max-sm:row-span-2 max-sm:row-start-1 max-sm:w-auto max-sm:h-full max-sm:flex-col',
              )}
              aria-hidden="true"
            >
              <span
                className={cn(
                  'h-0.5 flex-1 rounded-full transition-colors duration-300 ease-out-soft',
                  adaptive && 'max-sm:h-auto max-sm:w-0.5 max-sm:min-h-2',
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
                  adaptive && 'max-sm:h-auto max-sm:w-0.5 max-sm:min-h-2',
                  index === steps.length - 1 && 'invisible',
                  index < currentIndex ? 'bg-primary' : 'bg-border',
                )}
              />
            </div>

            {hasMeta ? (
              <span
                className={cn(
                  'min-h-4 px-1 text-center text-paragraph-mini text-foreground-subtle',
                  adaptive &&
                    'max-sm:col-start-2 max-sm:row-start-2 max-sm:self-start max-sm:px-0 max-sm:pb-2 max-sm:text-left max-sm:text-paragraph-xs',
                )}
              >
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
