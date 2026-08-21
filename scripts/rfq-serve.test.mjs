import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';

import { createRFQReportServer, latestDashboard, parseServeArgs } from './rfq-serve.mjs';

describe('rfq-serve.mjs', () => {
  it('parses the server options', () => {
    assert.deepEqual(parseServeArgs(['--port', '5000', '--host', 'localhost']), {
      port: '5000',
      host: 'localhost',
    });
  });

  it('finds and serves the latest dashboard', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-serve-'));
    const older = path.join(directory, 'older.html');
    const latest = path.join(directory, 'latest.html');
    fs.writeFileSync(older, '<h1>older</h1>');
    fs.writeFileSync(latest, '<h1>latest</h1>');
    fs.utimesSync(older, new Date(0), new Date(0));

    assert.equal(latestDashboard(directory), latest);

    const server = createRFQReportServer(directory);
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    const address = server.address();
    assert.equal(typeof address, 'object');
    try {
      const response = await fetch(`http://127.0.0.1:${address.port}/latest`);
      assert.equal(response.status, 200);
      assert.equal(await response.text(), '<h1>latest</h1>');
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });

  it('serves the QA Lab before the first evaluation', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-empty-'));
    const server = createRFQReportServer(directory);
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    const address = server.address();
    assert.equal(typeof address, 'object');
    try {
      const response = await fetch(`http://127.0.0.1:${address.port}/`);
      assert.equal(response.status, 200);
      const html = await response.text();
      assert.match(html, /Coti QA Lab/);
      const script = html.match(/<script>([\s\S]*)<\/script>/)?.[1];
      assert.ok(script);
      assert.doesNotThrow(() => new Function(script));
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });

  it('creates cases and requires confirmation before AI runs', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-api-'));
    const runs = new Map();
    const runManager = {
      get: (id) => runs.get(id) ?? null,
      start: (typeID, caseID) => {
        const run = {
          id: 'run-1',
          type_id: typeID,
          type_label: 'Custom',
          case_id: caseID,
          status: 'RUNNING',
          logs: [],
        };
        runs.set(run.id, run);
        return run;
      },
    };
    const server = createRFQReportServer(directory, { root: process.cwd(), runManager });
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    const address = server.address();
    assert.equal(typeof address, 'object');
    const base = `http://127.0.0.1:${address.port}`;
    try {
      const created = await fetch(`${base}/api/cases`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({ name: 'Cemento', message: 'Necesito 10 bolsas de cemento' }),
      });
      assert.equal(created.status, 201);
      const testCase = (await created.json()).case;

      const refused = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({ type_id: 'live_custom', case_id: testCase.id }),
      });
      assert.equal(refused.status, 409);

      const accepted = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({
          type_id: 'live_custom',
          case_id: testCase.id,
          confirm_ai: true,
        }),
      });
      assert.equal(accepted.status, 201);
      assert.equal((await accepted.json()).id, 'run-1');
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });
});
