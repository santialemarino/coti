/**
 * Creates the approximate index behind the catalog's semantic search, sized to the rows that
 * actually carry a vector. Kept apart from the executable so it can be imported and tested:
 * nothing here reads argv, prints, or exits.
 */

export const USAGE = 'Usage: node scripts/db-vector-index.mjs [--lists <n>]';

export const INDEX_NAME = 'idx_product_embedding';

/**
 * Reads the optional lists override, in either `--lists n` or `--lists=n` form, throwing a
 * message the caller prints as-is. Anything else is refused rather than ignored: this command
 * takes a write lock for minutes, and a dropped override is expensive to notice afterwards.
 */
export function parseArgs(argv) {
  let lists = null;

  for (let i = 0; i < argv.length; i++) {
    const [flag, inlineValue] = splitFlag(argv[i]);
    if (flag !== '--lists') throw new Error(`Unknown argument ${argv[i]}.\n${USAGE}`);

    const raw = inlineValue ?? argv[++i];
    lists = Number(raw);
    if (raw === undefined || raw === '' || !Number.isInteger(lists) || lists < 1) {
      throw new Error(`--lists takes a positive integer.\n${USAGE}`);
    }
  }
  return { lists };
}

/** Splits `--flag=value` into its two halves, leaving a bare `--flag` with no value. */
function splitFlag(argument) {
  const separator = argument.indexOf('=');
  if (separator === -1) return [argument, undefined];
  return [argument.slice(0, separator), argument.slice(separator + 1)];
}

/**
 * How many partitions the index is built with. pgvector's own guidance: rows/1000 while the
 * table is under a million rows, sqrt(rows) past that.
 */
export function listsFor(rows) {
  if (rows < 1) return 1;
  const lists = rows <= 1_000_000 ? Math.round(rows / 1000) : Math.round(Math.sqrt(rows));
  return Math.max(lists, 1);
}

/**
 * Rebuilds the index and reports what it did. Dropping first is what lets the command run again
 * after the catalog grows, with lists resized to it.
 *
 * Both statements go in one transaction, so a build that runs out of memory or is interrupted
 * leaves the working index in place instead of dropping it and putting nothing back. The build
 * takes a write lock on product for its duration, so this is a maintenance step rather than
 * something to run beside live traffic.
 */
export async function createVectorIndex(client, { lists } = { lists: null }) {
  const counted = await client.query(
    'SELECT count(*)::int AS rows FROM product WHERE embedding IS NOT NULL',
  );
  const embedded = counted.rows[0].rows;
  if (embedded === 0) {
    return { embedded, lists: null, created: false };
  }

  const partitions = lists ?? listsFor(embedded);
  await client.query('BEGIN');
  try {
    await client.query(`DROP INDEX IF EXISTS ${INDEX_NAME}`);
    // Interpolated because a WITH option takes no bind parameter, so the value is coerced to a
    // number here and nothing off the command line reaches the statement as text.
    await client.query(
      `CREATE INDEX ${INDEX_NAME} ON product USING ivfflat (embedding vector_cosine_ops) ` +
        `WITH (lists = ${Number(partitions)})`,
    );
    await client.query('COMMIT');
  } catch (err) {
    await client.query('ROLLBACK').catch(() => {});
    throw err;
  }
  return { embedded, lists: partitions, created: true };
}
