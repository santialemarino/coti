/**
 * Resolves DATABASE_ADMIN_URL — the owner role — for the scripts that are not request-scoped.
 * Falls back to apps/api/.env so a local run needs no exported variable.
 */
import fs from 'fs';
import path from 'path';

const KEY = 'DATABASE_ADMIN_URL';

export function loadOwnerUrl() {
  if (process.env[KEY]) return process.env[KEY];

  const envFile = path.join(process.cwd(), 'apps/api/.env');
  if (!fs.existsSync(envFile)) {
    console.error(`${KEY} not set and ${envFile} not found.`);
    process.exit(1);
  }
  const line = fs
    .readFileSync(envFile, 'utf8')
    .split('\n')
    .find((l) => l.startsWith(`${KEY}=`));
  if (!line) {
    console.error(`${KEY} not found in ${envFile}.`);
    process.exit(1);
  }
  return line
    .slice(`${KEY}=`.length)
    .trim()
    .replace(/^["']|["']$/g, '');
}
