/**
 * Creates the approximate index behind the catalog's semantic search, sized to the rows that
 * actually carry a vector. Kept apart from the executable so it can be imported and tested:
 * nothing here reads argv, prints, or exits.
 */

export const USAGE = 'Usage: node scripts/db-vector-index.mjs [--lists <n>]';

export const INDEX_NAME = 'idx_product_embedding';

/** Reads the optional lists override, throwing a message the caller prints as-is. */
export function parseArgs(argv) {
  const flag = argv.indexOf('--lists');
  if (flag === -1) return { lists: null };

  const lists = Number(argv[flag + 1]);
  if (!Number.isInteger(lists) || lists < 1) {
    throw new Error(`--lists takes a positive integer.\n${USAGE}`);
  }
  return { lists };
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
 * The build takes a write lock on product for its duration, so it is a maintenance step rather
 * than something to run beside live traffic.
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
  await client.query(`DROP INDEX IF EXISTS ${INDEX_NAME}`);
  // Interpolated because a WITH option takes no bind parameter, so the value is coerced to a
  // number here and nothing off the command line reaches the statement as text.
  await client.query(
    `CREATE INDEX ${INDEX_NAME} ON product USING ivfflat (embedding vector_cosine_ops) ` +
      `WITH (lists = ${Number(partitions)})`,
  );
  return { embedded, lists: partitions, created: true };
}
