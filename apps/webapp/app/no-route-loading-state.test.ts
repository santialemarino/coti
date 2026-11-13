import { readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

/*
 * No customer screen has a route loading state. A navigation keeps the old screen until the new one is
 * ready; a `loading.tsx` blanks it the moment a navigation starts, React then holds the placeholder
 * ~300 ms however soon the page is ready, and a placeholder that misses the page's geometry jumps.
 */

const APP = dirname(fileURLToPath(import.meta.url));
const LOADING_FILE = /^loading\.(tsx|ts|jsx|js)$/;

function directoriesUnder(dir: string): string[] {
  const out = [relative(APP, dir) || '.'];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...directoriesUnder(full));
  }
  return out;
}

const SCANNED = directoriesUnder(APP);

describe('route loading states', () => {
  it('scans app/ and every directory under it', () => {
    expect(SCANNED).toContain('.');
    expect(SCANNED).toContain('profile');
    expect(SCANNED).toContain('quotes/[token]');
    expect(SCANNED.length).toBeGreaterThan(20);
  });

  it('exist nowhere in the app', () => {
    const found = SCANNED.flatMap((dir) =>
      readdirSync(join(APP, dir))
        .filter((entry) => LOADING_FILE.test(entry))
        .map((entry) => join(dir, entry)),
    );
    expect(
      found,
      'a route loading state blanks the screen on every navigation to it and holds it ~300 ms',
    ).toEqual([]);
  });
});
