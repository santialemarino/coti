import type { Metadata } from 'next';
import { getTranslations } from 'next-intl/server';

// Reads the meta namespace so a tab title is copy like any other, not a literal
// sitting in a .tsx file.
export async function generatePageMetadata(key: string): Promise<Metadata> {
  const t = await getTranslations('meta');
  return { title: `${t(`${key}.title`)} — Coti`, description: t(`${key}.description`) };
}
