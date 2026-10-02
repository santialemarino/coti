import * as React from 'react';

import { cn } from '../lib/utils';

interface MetaListProps {
  /*
   * The facts to line up. Anything empty — `null`, `undefined`, `false`, `''` — is dropped along
   * with the separator that would have preceded it.
   */
  items: React.ReactNode[];
  /* Keep to one line: every fact holds its width and only the last one truncates. */
  singleLine?: boolean;
  className?: string;
}

/*
 * A single line of metadata under a title: a date, a channel, an owner, a branch. The separator is
 * drawn here rather than typed at the call site, because a call site that writes its own ends up
 * rendering "10 de sept · · Morón" the moment one of the facts is missing — the separator is only
 * ever correct as a function of what survived, never of what was written.
 *
 * Emptiness has to reach this list unwrapped. An element around an empty string is still an element,
 * so `<span>{name}</span>` with no name keeps its separator; pass `name ? <span>…</span> : null`.
 */
function MetaList({ items, singleLine = false, className }: MetaListProps) {
  const shown = items.filter(
    (item) => item !== null && item !== undefined && item !== false && item !== '',
  );

  return (
    <div
      data-slot="meta-list"
      className={cn(
        'flex items-center gap-x-1.5 gap-y-1 text-paragraph-sm text-foreground-muted',
        singleLine ? 'min-w-0 flex-nowrap' : 'flex-wrap',
        className,
      )}
    >
      {shown.map((item, index) => {
        const last = index === shown.length - 1;
        return (
          <React.Fragment key={index}>
            {index > 0 ? (
              <span
                aria-hidden="true"
                className={cn('text-foreground-subtle', singleLine && 'shrink-0')}
              >
                ·
              </span>
            ) : null}
            <span
              className={cn(
                'inline-flex items-center gap-x-1',
                singleLine && (last ? 'min-w-0' : 'shrink-0 whitespace-nowrap'),
              )}
            >
              {singleLine && last ? <span className="truncate">{item}</span> : item}
            </span>
          </React.Fragment>
        );
      })}
    </div>
  );
}

export { MetaList };
