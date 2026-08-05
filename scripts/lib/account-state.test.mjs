import assert from 'node:assert/strict';
import { describe, it } from 'node:test';

import { parseArgs } from './account-state.mjs';

// Argument reading is the half that needs no database, and the half a typo in the command line
// reaches first: a script that flips the wrong account is worse than one that refuses to run.

const ID = 'a0000000-0000-4000-8000-000000000001';

describe('parseArgs', () => {
  it('reads both actions', () => {
    assert.deepEqual(parseArgs(['deactivate', '--account', ID]), { activate: false, id: ID });
    assert.deepEqual(parseArgs(['activate', '--account', ID]), { activate: true, id: ID });
  });

  it('accepts an uppercase uuid', () => {
    assert.equal(parseArgs(['activate', '--account', ID.toUpperCase()]).id, ID.toUpperCase());
  });

  for (const [name, argv] of [
    ['no arguments', []],
    ['an unknown action', ['disable', '--account', ID]],
    ['a missing --account flag', ['deactivate', ID]],
    ['--account with no value', ['deactivate', '--account']],
    ['a value that is not a uuid', ['deactivate', '--account', 'not-a-uuid']],
    ['a bare number', ['deactivate', '--account', '42']],
    ['an id missing its dashes', ['deactivate', '--account', ID.replaceAll('-', '')]],
  ]) {
    it(`refuses ${name}`, () => {
      assert.throws(() => parseArgs(argv));
    });
  }

  // The action is read positionally, so an id that happens to look like one must not be
  // mistaken for it.
  it('does not accept the uuid in the action position', () => {
    assert.throws(() => parseArgs([ID, '--account', ID]));
  });
});
