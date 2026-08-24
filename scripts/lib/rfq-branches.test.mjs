import assert from 'node:assert/strict';
import os from 'node:os';
import { describe, it } from 'node:test';

import { createRFQBranchManager } from './rfq-branches.mjs';

const VILLA_BOSCH = 'b0000000-0000-4000-8000-000000000001';
const MORON = 'b0000000-0000-4000-8000-000000000002';

describe('rfq-branches.mjs', () => {
  it('authenticates and lists only active branches', async () => {
    const calls = [];
    const manager = createRFQBranchManager({
      root: os.tmpdir(),
      apiURL: 'http://api.test',
      environment: {},
      fetchImpl: async (url, options) => {
        calls.push({ url, options });
        if (url.endsWith('/login')) return jsonResponse(200, { access_token: 'test-token' });
        return jsonResponse(200, {
          items: [
            { id: VILLA_BOSCH, name: 'Villa Bosch', is_active: true },
            { id: MORON, name: 'Moron', is_active: false },
          ],
        });
      },
    });

    const result = await manager.list();

    assert.deepEqual(result, {
      items: [{ id: VILLA_BOSCH, name: 'Villa Bosch' }],
      default_branch_id: VILLA_BOSCH,
    });
    assert.equal(calls[1].options.headers.Authorization, 'Bearer test-token');
  });

  it('uses the first available branch when the configured default is unavailable', async () => {
    const manager = createRFQBranchManager({
      root: os.tmpdir(),
      apiURL: 'http://api.test',
      environment: {},
      fetchImpl: async (url) =>
        url.endsWith('/login')
          ? jsonResponse(200, { access_token: 'test-token' })
          : jsonResponse(200, {
              items: [{ id: MORON, name: 'Moron', is_active: true }],
            }),
    });

    assert.equal((await manager.list()).default_branch_id, MORON);
  });

  it('does not expose credentials when authentication fails', async () => {
    const manager = createRFQBranchManager({
      root: os.tmpdir(),
      apiURL: 'http://api.test',
      environment: {},
      fetchImpl: async () => jsonResponse(401, { error: 'secret credential echoed by API' }),
    });

    await assert.rejects(manager.list(), (error) => {
      assert.match(error.message, /HTTP 401/);
      assert.doesNotMatch(error.message, /secret credential/);
      return true;
    });
  });
});

function jsonResponse(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}
