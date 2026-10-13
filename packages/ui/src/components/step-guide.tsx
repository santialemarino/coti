import * as React from 'react';
import { CheckIcon } from 'lucide-react';

import { cn } from '../lib/utils';

export type StepGuideStatus = 'done' | 'current' | 'pending';

export interface StepGuideStep {
  id: string;
  title: string;
  status: StepGuideStatus;
  /* What the step asks for: its instructions and the control that completes it. */
  children?: React.ReactNode;
}

interface StepGuideProps {
  steps: StepGuideStep[];
  /* Read after each title, so the state is never carried by colour alone. */
  statusLabels: Record<StepGuideStatus, string>;
  className?: string;
}

/*
 * A task walked through top to bottom, where each step carries its own action and its state comes
 * from data the app tracks. Unlike `Stepper` several steps can be current at once — the ones the
 * caller can act on now — and unlike `StepList` every step holds content, not a one-line label.
 *
 * Only the first current step is `aria-current`, so assistive technology announces one position.
 */
function StepGuide({ steps, statusLabels, className }: StepGuideProps) {
  const firstCurrent = steps.findIndex((step) => step.status === 'current');

  return (
    <ol data-slot="step-guide" className={cn('flex flex-col', className)}>
      {steps.map((step, index) => {
        const last = index === steps.length - 1;
        return (
          <li
            key={step.id}
            data-status={step.status}
            aria-current={index === firstCurrent ? 'step' : undefined}
            className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4"
          >
            <div aria-hidden="true" className="flex flex-col items-center">
              <span
                className={cn(
                  'grid size-7 shrink-0 place-items-center border-2 rounded-full text-paragraph-xs-semibold',
                  'transition-[background-color,border-color,box-shadow,color] duration-300 ease-out-soft',
                  step.status === 'done' && 'bg-primary border-primary text-primary-foreground',
                  step.status === 'current' &&
                    'bg-background border-primary shadow-e1 ring-3 ring-ring/25 text-primary',
                  step.status === 'pending' && 'bg-background border-border text-foreground-subtle',
                )}
              >
                {step.status === 'done' ? <CheckIcon className="size-3.5" /> : index + 1}
              </span>
              {last ? null : (
                <span
                  className={cn(
                    'w-0.5 flex-1 min-h-4 my-1 rounded-full transition-colors duration-300 ease-out-soft',
                    step.status === 'done' ? 'bg-primary' : 'bg-border',
                  )}
                />
              )}
            </div>
            <div className={cn('flex flex-col min-w-0 pt-0.5 gap-y-3', !last && 'pb-8')}>
              <h3
                className={cn(
                  'text-paragraph-semibold',
                  step.status === 'pending' ? 'text-foreground-muted' : 'text-foreground',
                )}
              >
                {step.title}
                <span className="sr-only">. {statusLabels[step.status]}</span>
              </h3>
              {step.children}
            </div>
          </li>
        );
      })}
    </ol>
  );
}

export { StepGuide };
