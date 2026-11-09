'use client';

import { createContext, useContext, type ComponentProps } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';

import { portalPath, portalSlug } from '@/lib/portal/routing';

export const PortalSlugContext = createContext('');

export function usePortalPath() {
  const slug = useContext(PortalSlugContext);
  return (path: string) => portalPath(path, slug);
}

export function PortalLink(props: ComponentProps<typeof Link>) {
  const slug = useContext(PortalSlugContext);
  return (
    <Link
      {...props}
      href={typeof props.href === 'string' ? portalPath(props.href, slug) : props.href}
    />
  );
}

export function PortalProvider({ slug, children }: { slug: string; children: React.ReactNode }) {
  const pathname = usePathname();
  const match = /^\/c\/([^/]+)(?:\/|$)/.exec(pathname || '');
  const current = portalSlug(match?.[1] ?? null) || slug;
  return <PortalSlugContext.Provider value={current}>{children}</PortalSlugContext.Provider>;
}
