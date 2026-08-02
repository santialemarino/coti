import type { Metadata } from 'next';

export function generatePageMetadata(title: string, description: string): Metadata {
  return { title: `${title} — Coti`, description };
}
