import { useSyncExternalStore } from 'react';

/*
 * Whether a media query matches, kept in step with the viewport. The server cannot know, so it and
 * the first client render answer `serverValue`, and the real answer follows straight after.
 */
export function useMediaQuery(query: string, serverValue = false): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const list = window.matchMedia(query);
      list.addEventListener('change', onChange);
      return () => list.removeEventListener('change', onChange);
    },
    () => window.matchMedia(query).matches,
    () => serverValue,
  );
}
