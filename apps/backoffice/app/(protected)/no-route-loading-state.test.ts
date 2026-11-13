import { readdirSync, statSync } from 'node:fs';
import { dirname, join, relative } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

/*
 * No signed-in screen has a route loading state. A navigation keeps the old screen until the new one
 * is ready, answered by `NavigationPendingProvider`; a `loading.tsx` blanks it the moment a navigation
 * starts and React then holds the placeholder ~300 ms however soon the page is ready. The walk covers
 * `app/` itself (a root boundary wraps every group) and the whole protected tree.
 */

const PROTECTED = dirname(fileURLToPath(import.meta.url));
const APP = dirname(PROTECTED);
const LOADING_FILE = /^loading\.(tsx|ts|jsx|js)$/;

function directoriesUnder(dir: string): string[] {
  const out = [relative(APP, dir)];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out.push(...directoriesUnder(full));
  }
  return out;
}

const SCANNED = ['.', ...directoriesUnder(PROTECTED)];

describe('route loading states', () => {
  it('scans app/ and every directory of the protected tree', () => {
    expect(SCANNED).toContain('(protected)/reports');
    expect(SCANNED).toContain('(protected)/settings/invoicing/arca');
    expect(SCANNED.length).toBeGreaterThan(40);
  });

  it('exist nowhere a protected screen renders', () => {
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
