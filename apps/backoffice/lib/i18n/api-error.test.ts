import { createTranslator } from 'next-intl';
import { describe, expect, it } from 'vitest';

import { API_ERROR_CODES } from '@/lib/api/errors';
import { apiErrorMessage, type CatalogReader } from '@/lib/i18n/api-error';
import messages from '@/translations/es.json';

/*
 * The schema's own catalog, which is keyed by rejection rather than by API code: `tooLong`,
 * `invalidEmail`. It is never reached through a flow namespace, so it is the one `errors` object
 * the code-key rule does not govern.
 */
const SCHEMA_CATALOG = 'common.form.errors';

function reader(): CatalogReader {
  return createTranslator({ locale: 'es', messages }) as unknown as CatalogReader;
}

describe('apiErrorMessage', () => {
  it('words a code from the shared catalog when no flow overrides it', () => {
    expect(apiErrorMessage(reader(), 'branches', 'FORBIDDEN')).toBe(messages.errors.FORBIDDEN);
  });

  it('lets a flow override the wording of a code it says differently', () => {
    expect(apiErrorMessage(reader(), 'auth.login', 'UNAUTHENTICATED')).toBe(
      messages.auth.login.errors.UNAUTHENTICATED,
    );
    expect(messages.auth.login.errors.UNAUTHENTICATED).not.toBe(messages.errors.UNAUTHENTICATED);
  });

  /*
   * The whole reason the namespace is walked back a segment at a time: mailing a recovery link
   * reads a 422 as "that user is deactivated" while the rest of the flow reads it as "check these
   * values", and neither action should have to branch on the code to say so.
   */
  it('prefers the most specific namespace over the ones above it', () => {
    expect(apiErrorMessage(reader(), 'users.passwordReset', 'INVALID_INPUT')).toBe(
      messages.users.passwordReset.errors.INVALID_INPUT,
    );
    expect(apiErrorMessage(reader(), 'users', 'INVALID_INPUT')).toBe(
      messages.users.errors.INVALID_INPUT,
    );
    expect(messages.users.passwordReset.errors.INVALID_INPUT).not.toBe(
      messages.users.errors.INVALID_INPUT,
    );
  });

  /*
   * The half a single lookup would miss: an action inherits its flow's wording for every code it
   * does not word itself, so mailing a link still reports a user that is gone the way the list
   * does rather than dropping to the shared "no encontramos lo que buscabas".
   */
  it('inherits the wording of the namespace above before reaching the shared catalog', () => {
    expect(apiErrorMessage(reader(), 'users.passwordReset', 'NOT_FOUND')).toBe(
      messages.users.errors.NOT_FOUND,
    );
    expect(messages.users.errors.NOT_FOUND).not.toBe(messages.errors.NOT_FOUND);
    // Nothing at either level words it, so the shared catalog does.
    expect(apiErrorMessage(reader(), 'users.passwordReset', 'FORBIDDEN')).toBe(
      messages.errors.FORBIDDEN,
    );
  });

  it('reads a failure that carried no code as the unexpected one', () => {
    expect(apiErrorMessage(reader(), 'branches')).toBe(messages.branches.errors.INTERNAL);
    expect(apiErrorMessage(reader(), undefined)).toBe(messages.errors.INTERNAL);
  });

  it('words every code for every namespace a screen binds', () => {
    const namespaces = [
      undefined,
      'auth.login',
      'auth.signup',
      'auth.forgotPassword',
      'auth.resetPassword',
      'auth.changePassword',
      'auth.verifyEmail',
      'auth.verifyEmail.resend',
      'account',
      'branches',
      'users',
      'users.passwordReset',
      'priceImport',
      'priceImport.export',
    ];
    namespaces.forEach((namespace) => {
      API_ERROR_CODES.forEach((code) => {
        expect(apiErrorMessage(reader(), namespace, code)).toBeTruthy();
      });
    });
  });
});

/*
 * An override is a code repeated under a flow's own `errors`, so a typo silently falls through to
 * the shared wording and the screen keeps working while saying the wrong thing. Nothing else
 * catches it: the key is a string on both sides.
 */
describe('the catalog overrides', () => {
  it('names only codes the wire can produce', () => {
    const stray: string[] = [];
    const walk = (node: unknown, path: string[]) => {
      if (typeof node !== 'object' || node === null) return;
      Object.entries(node as Record<string, unknown>).forEach(([key, value]) => {
        const here = [...path, key];
        if (key === 'errors' && here.join('.') !== SCHEMA_CATALOG) {
          Object.keys(value as Record<string, unknown>)
            .filter((code) => !API_ERROR_CODES.some((known) => known === code))
            .forEach((code) => stray.push([...here, code].join('.')));
          return;
        }
        walk(value, here);
      });
    };
    walk(messages, []);
    expect(stray).toEqual([]);
  });
});
