'use client';

import { useEffect, useRef, useTransition } from 'react';
import { useRouter } from 'next/navigation';

import { useNavigationPending } from '@/hooks/use-navigation-pending';

type NavigateOptions = Parameters<ReturnType<typeof useRouter>['push']>[1];

/*
 * `router.push` / `router.replace` for code that navigates without a link click, so it gets the same
 * pending feedback a click does. A navigation whose transition ends without moving the URL is settled.
 */
export function usePendingRouter() {
  const router = useRouter();
  const { begin, settle } = useNavigationPending();
  const [isPending, startTransition] = useTransition();
  const started = useRef<number[]>([]);

  useEffect(() => {
    if (isPending || started.current.length === 0) return;
    for (const id of started.current) settle(id);
    started.current = [];
  }, [isPending, settle]);

  function navigate(method: 'push' | 'replace', href: string, options?: NavigateOptions) {
    started.current.push(begin(href));
    startTransition(() => router[method](href, options));
  }

  return {
    push: (href: string, options?: NavigateOptions) => navigate('push', href, options),
    replace: (href: string, options?: NavigateOptions) => navigate('replace', href, options),
    isPending,
  };
}
