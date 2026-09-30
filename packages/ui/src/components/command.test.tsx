import { createRef } from 'react';
import { render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { Command, CommandItem, CommandList } from './command';

function Harness({ query, listRef }: { query: string; listRef?: React.Ref<HTMLDivElement> }) {
  return (
    <Command>
      <CommandList ref={listRef}>
        <CommandItem value={query}>{query}</CommandItem>
      </CommandList>
    </Command>
  );
}

describe('CommandList', () => {
  afterEach(() => vi.unstubAllGlobals());

  // A searchable list re-renders on every keystroke; its scroll-lane observers must survive that.
  it('keeps one set of observers across re-renders', () => {
    // cmdk runs observers of its own, so only the ones watching the list itself are counted.
    const watchingList = new Set<object>();
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe(target: Element) {
          if (target.getAttribute('data-slot') === 'command-list') watchingList.add(this);
        }
        unobserve() {}
        disconnect() {}
      },
    );
    const view = render(<Harness query="a" />);
    view.rerender(<Harness query="ar" />);
    view.rerender(<Harness query="arg" />);

    expect(watchingList.size).toBe(1);
  });

  it('releases a forwarded ref when it unmounts', () => {
    const listRef = createRef<HTMLDivElement>();
    const view = render(<Harness query="a" listRef={listRef} />);
    expect(listRef.current).not.toBeNull();

    view.unmount();

    expect(listRef.current).toBeNull();
  });
});
