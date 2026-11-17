'use client';

import { useEffect, useLayoutEffect, useRef } from 'react';

import { RFQ_REFRESH_INTERVAL_SECONDS } from '@/lib/config';

export function useQueuePolling(refresh: () => Promise<void>, enabled: boolean) {
  const refreshRef = useRef(refresh);

  useLayoutEffect(() => {
    refreshRef.current = refresh;
  }, [refresh]);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    let pending = false;
    let timer: ReturnType<typeof setTimeout>;

    async function poll() {
      if (!active || pending) return;
      clearTimeout(timer);
      pending = true;
      try {
        if (document.visibilityState === 'visible') await refreshRef.current();
      } catch {
        // A failed background read retries without replacing the last usable screen.
      } finally {
        pending = false;
        if (active) timer = setTimeout(() => void poll(), RFQ_REFRESH_INTERVAL_SECONDS * 1000);
      }
    }

    function onVisible() {
      if (document.visibilityState === 'visible') void poll();
    }

    timer = setTimeout(() => void poll(), RFQ_REFRESH_INTERVAL_SECONDS * 1000);
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      active = false;
      clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [enabled]);
}
