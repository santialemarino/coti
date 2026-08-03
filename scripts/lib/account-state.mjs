/**
 * Flips account.is_active, the flag login, refresh and tenant resolution all read. Kept apart
 * from the executable so it can be imported and tested: nothing here reads argv, prints, or
 * exits.
 */

export const USAGE = 'Usage: node scripts/db-account.mjs <deactivate | activate> --account <uuid>';

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Reads the action and the account id, throwing a message the caller prints as-is. */
export function parseArgs(argv) {
  const [action, ...rest] = argv;
  if (action !== 'deactivate' && action !== 'activate') throw new Error(USAGE);

  const flag = rest.indexOf('--account');
  const id = flag === -1 ? '' : rest[flag + 1];
  if (!id || !UUID_PATTERN.test(id)) throw new Error(`--account takes an account uuid.\n${USAGE}`);

  return { activate: action === 'activate', id };
}

/** Applies the flag in one transaction, returning what it found or null for an unknown id. */
export async function setAccountActive(client, { activate, id }) {
  await client.query('BEGIN');
  try {
    // FOR UPDATE so two runs against the same account serialize rather than both reporting
    // the state they read.
    const found = await client.query(
      'SELECT name, is_active FROM account WHERE id = $1 FOR UPDATE',
      [id],
    );
    if (found.rowCount === 0) {
      await client.query('ROLLBACK');
      return null;
    }

    await client.query('UPDATE account SET is_active = $2 WHERE id = $1', [id, activate]);

    let usersCut = 0;
    if (!activate) {
      // Defence in depth, not what cuts access: the live account check already refuses every
      // request. What the bump adds is that a token minted before the closure stays dead once
      // the account is reopened, instead of coming back for the rest of its lifetime.
      const bumped = await client.query(
        'UPDATE app_user SET session_epoch = session_epoch + 1 WHERE account_id = $1',
        [id],
      );
      usersCut = bumped.rowCount;
    }

    await client.query('COMMIT');
    return { name: found.rows[0].name, wasActive: found.rows[0].is_active, usersCut };
  } catch (err) {
    await client.query('ROLLBACK').catch(() => {});
    throw err;
  }
}
