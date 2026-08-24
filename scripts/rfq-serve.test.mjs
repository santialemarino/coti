import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { describe, it } from 'node:test';

import { createRFQReportServer, latestDashboard, parseServeArgs } from './rfq-serve.mjs';

const BRANCH_ID = 'b0000000-0000-4000-8000-000000000001';

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
      assert.match(html, /id="branch-select"/);
      assert.match(html, /id="delete-case"/);
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
      isCaseRunning: (caseID) =>
        [...runs.values()].some((run) => run.case_id === caseID && run.status === 'RUNNING'),
      start: (typeID, caseID, options) => {
        const run = {
          id: 'run-1',
          type_id: typeID,
          type_label: 'Custom',
          case_id: caseID,
          branch_id: options.branchID,
          status: 'RUNNING',
          logs: [],
        };
        runs.set(run.id, run);
        return run;
      },
    };
    const preflightManager = readyPreflightManager();
    const branchManager = readyBranchManager();
    const server = createRFQReportServer(directory, {
      root: process.cwd(),
      runManager,
      preflightManager,
      branchManager,
    });
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

      const missingBranch = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({
          type_id: 'live_custom',
          case_id: testCase.id,
          confirm_ai: true,
        }),
      });
      assert.equal(missingBranch.status, 400);

      const unavailableBranch = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({
          type_id: 'live_custom',
          case_id: testCase.id,
          branch_id: 'b0000000-0000-4000-8000-000000000002',
          confirm_ai: true,
        }),
      });
      assert.equal(unavailableBranch.status, 409);

      const accepted = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({
          type_id: 'live_custom',
          case_id: testCase.id,
          branch_id: BRANCH_ID,
          confirm_ai: true,
        }),
      });
      assert.equal(accepted.status, 201);
      const acceptedRun = await accepted.json();
      assert.equal(acceptedRun.id, 'run-1');
      assert.equal(acceptedRun.branch_id, BRANCH_ID);

      const runningDelete = await fetch(`${base}/api/cases/${testCase.id}`, {
        method: 'DELETE',
        headers: { Origin: base },
      });
      assert.equal(runningDelete.status, 409);

      runs.get('run-1').status = 'PASSED';
      const deleted = await fetch(`${base}/api/cases/${testCase.id}`, {
        method: 'DELETE',
        headers: { Origin: base },
      });
      assert.equal(deleted.status, 200);
      assert.equal((await deleted.json()).deleted_case.id, testCase.id);
      assert.deepEqual(loadCases(directory), []);
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });

  it('publishes readiness and blocks a run that fails preflight', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-preflight-'));
    let starts = 0;
    const blocked = {
      ready: false,
      checked_at: '2026-08-24T12:00:00.000Z',
      checks: [
        {
          id: 'database',
          label: 'PostgreSQL accesible',
          status: 'BLOCKED',
          detail: 'No connection',
        },
      ],
    };
    const preflightManager = {
      all: async () => ({ pipeline_integration: blocked, live_suite: blocked }),
      forType: async () => blocked,
    };
    const runManager = {
      get: () => null,
      start: () => {
        starts += 1;
        return {};
      },
    };
    const server = createRFQReportServer(directory, {
      root: process.cwd(),
      runManager,
      preflightManager,
    });
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    const address = server.address();
    assert.equal(typeof address, 'object');
    const base = `http://127.0.0.1:${address.port}`;
    try {
      const stateResponse = await fetch(`${base}/api/state`);
      const state = await stateResponse.json();
      assert.equal(state.preflights.pipeline_integration.ready, false);
      assert.equal(state.api_online, false);

      const response = await fetch(`${base}/api/runs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Origin: base },
        body: JSON.stringify({ type_id: 'pipeline_integration' }),
      });
      assert.equal(response.status, 409);
      assert.match((await response.json()).error, /PostgreSQL accesible/);
      assert.equal(starts, 0);
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });

  it('publishes branches available to live evaluations', async () => {
    const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'coti-rfq-branches-'));
    const server = createRFQReportServer(directory, {
      root: process.cwd(),
      preflightManager: readyPreflightManager(),
      branchManager: readyBranchManager(),
    });
    await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
    const address = server.address();
    assert.equal(typeof address, 'object');
    try {
      const response = await fetch(`http://127.0.0.1:${address.port}/api/state`);
      const state = await response.json();

      assert.deepEqual(state.branches, [{ id: BRANCH_ID, name: 'Villa Bosch' }]);
      assert.equal(state.default_branch_id, BRANCH_ID);
      assert.equal(state.branches_error, null);
    } finally {
      await new Promise((resolve, reject) =>
        server.close((error) => (error ? reject(error) : resolve())),
      );
    }
  });
});

function readyPreflightManager() {
  const ready = {
    ready: true,
    checked_at: '2026-08-24T12:00:00.000Z',
    checks: [{ id: 'api', label: 'API RFQ disponible', status: 'READY', detail: 'Ready' }],
  };
  return {
    all: async () => ({ live_suite: ready, live_custom: ready }),
    forType: async () => ready,
  };
}

function readyBranchManager() {
  return {
    list: async () => ({
      items: [{ id: BRANCH_ID, name: 'Villa Bosch' }],
      default_branch_id: BRANCH_ID,
    }),
  };
}

function loadCases(directory) {
  return JSON.parse(fs.readFileSync(path.join(directory, 'custom-cases.json'), 'utf8'));
}
