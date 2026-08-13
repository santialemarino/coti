import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { createVectorIndex, listsFor, parseArgs } from './vector-index.mjs';

// The sizing and the argument reading are pure, so they are tested without a database. The
// statements themselves are checked through a stand-in client, which is what proves the count
// of zero stops before it builds an index over nothing.

function fakeClient(embedded) {
  const statements = [];
  return {
    statements,
    async query(sql) {
      statements.push(sql);
      if (sql.startsWith('SELECT count')) return { rows: [{ rows: embedded }] };
      return { rows: [] };
    },
  };
}

describe('listsFor', () => {
  it('follows rows/1000 up to a million rows', () => {
    assert.equal(listsFor(50_000), 50);
    assert.equal(listsFor(1_000_000), 1000);
  });

  it('switches to the square root past a million', () => {
    assert.equal(listsFor(4_000_000), 2000);
  });

  // A catalog smaller than a thousand products would otherwise round to zero partitions, which
  // is not an index the database will build.
  it('never drops below one partition', () => {
    assert.equal(listsFor(0), 1);
    assert.equal(listsFor(120), 1);
  });
});

describe('parseArgs', () => {
  it('defaults to sizing the index from the catalog', () => {
    assert.deepEqual(parseArgs([]), { lists: null });
  });

  it('takes an operator override', () => {
    assert.deepEqual(parseArgs(['--lists', '32']), { lists: 32 });
  });

  it('refuses anything that is not a positive whole number', () => {
    for (const bad of [['--lists'], ['--lists', '0'], ['--lists', '-4'], ['--lists', 'many']]) {
      assert.throws(() => parseArgs(bad), /--lists takes a positive integer/);
    }
  });
});

describe('createVectorIndex', () => {
  // An index over a catalog with no vectors is degenerate, which is the whole reason this is a
  // command rather than a migration.
  it('builds nothing when no product carries an embedding', async () => {
    const client = fakeClient(0);

    const result = await createVectorIndex(client);

    assert.deepEqual(result, { embedded: 0, lists: null, created: false });
    assert.equal(
      client.statements.filter((s) => s.includes('CREATE INDEX')).length,
      0,
      'no index should be built',
    );
  });

  it('sizes the index to the rows that carry a vector', async () => {
    const client = fakeClient(12_000);

    const result = await createVectorIndex(client);

    assert.deepEqual(result, { embedded: 12_000, lists: 12, created: true });
    assert.match(client.statements.at(-1), /USING ivfflat \(embedding vector_cosine_ops\)/);
    assert.match(client.statements.at(-1), /WITH \(lists = 12\)/);
  });

  // Rebuilding is what lets the command run again once the catalog has grown.
  it('drops the previous index before building the new one', async () => {
    const client = fakeClient(3000);

    await createVectorIndex(client, { lists: 7 });

    const dropped = client.statements.findIndex((s) => s.includes('DROP INDEX'));
    const created = client.statements.findIndex((s) => s.includes('CREATE INDEX'));
    assert.ok(dropped !== -1 && created !== -1, 'both statements should run');
    assert.ok(dropped < created, 'the drop has to come first');
    assert.match(client.statements[created], /WITH \(lists = 7\)/);
  });
});
