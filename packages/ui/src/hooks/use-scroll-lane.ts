'use client';

import * as React from 'react';

/*
 * A callback ref that marks its element `data-scroll-lane` while its content overflows, so the
 * `scroll-lane` utility can clear an overlay scrollbar's thumb only when there is one to clear.
 */
function useScrollLane<T extends HTMLElement>() {
  return React.useCallback((element: T | null) => {
    if (!element) return;
    const update = () =>
      element.toggleAttribute('data-scroll-lane', element.scrollHeight > element.clientHeight);
    const resize = new ResizeObserver(update);
    const observeChildren = () => [...element.children].forEach((child) => resize.observe(child));
    const mutations = new MutationObserver(() => {
      observeChildren();
      update();
    });
    resize.observe(element);
    observeChildren();
    mutations.observe(element, { childList: true });
    update();
    return () => {
      resize.disconnect();
      mutations.disconnect();
    };
  }, []);
}

export { useScrollLane };
