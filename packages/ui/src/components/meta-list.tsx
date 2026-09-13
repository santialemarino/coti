import * as React from 'react';

import { cn } from '../lib/utils';

interface MetaListProps {
  /*
   * The facts to line up. Anything empty — `null`, `undefined`, `false`, `''` — is dropped along
   * with the separator that would have preceded it.
   */
  items: React.ReactNode[];
  className?: string;
}

/*
 * A single line of metadata under a title: a date, a channel, an owner, a branch. The separator is
 * drawn here rather than typed at the call site, because a call site that writes its own ends up
 * rendering "10 de sept · · Morón" the moment one of the facts is missing — the separator is only
 * ever correct as a function of what survived, never of what was written.
 */
function MetaList({ items, className }: MetaListProps) {
  const shown = items.filter(
    (item) => item !== null && item !== undefined && item !== false && item !== '',
  );

  return (
    <div
      data-slot="meta-list"
      className={cn(
        'flex flex-wrap items-center gap-x-1.5 gap-y-1 text-paragraph-sm text-foreground-muted',
        className,
      )}
    >
      {shown.map((item, index) => (
        <React.Fragment key={index}>
          {index > 0 ? (
            <span aria-hidden="true" className="text-foreground-subtle">
              ·
            </span>
          ) : null}
          <span className="inline-flex items-center gap-x-1">{item}</span>
        </React.Fragment>
      ))}
    </div>
  );
}

export { MetaList };
