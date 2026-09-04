/**
 * Resolves the database URLs the operational scripts need — the owner role they connect as, and
 * the app role's, which carries the password a fresh database still has to be given.
 * Falls back to apps/api/.env so a local run needs no exported variable.
 */
import fs from 'fs';
import path from 'path';

export function loadAppUrl() {
  return loadUrl('DATABASE_URL');
}

export function loadOwnerUrl() {
  return loadUrl('DATABASE_ADMIN_URL');
}

function loadUrl(key) {
  if (process.env[key]) return process.env[key];

  const envFile = path.join(process.cwd(), 'apps/api/.env');
  if (!fs.existsSync(envFile)) {
    console.error(`${key} not set and ${envFile} not found.`);
    process.exit(1);
  }
  const line = fs
    .readFileSync(envFile, 'utf8')
    .split('\n')
    .find((l) => l.startsWith(`${key}=`));
  if (!line) {
    console.error(`${key} not found in ${envFile}.`);
    process.exit(1);
  }
  return line
    .slice(`${key}=`.length)
    .trim()
    .replace(/^["']|["']$/g, '');
}
