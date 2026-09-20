import { test, afterEach } from 'node:test';
import assert from 'node:assert/strict';
import { discoveryCount, inventoryNotice, latestInventory, liveOnu } from '../src/api.ts';

const originalFetch = globalThis.fetch;
afterEach(() => { globalThis.fetch = originalFetch; });
const response = (data) => new Response(JSON.stringify({ data }), { status: 200 });
const run = (id, onus_sampled, extra = {}) => ({
  id, onus_sampled, device_id: 'example', status: 'ok', pons_error: 0, pons_ok: 1,
  started_at: '2026-01-01T00:00:00Z', finished_at: '2026-01-01T00:01:00Z', ...extra,
});

test('newest finished run wins even when smaller or failed; never falls back to a larger old inventory', async () => {
  const calls = [];
  globalThis.fetch = async (url, options) => {
    calls.push(url);
    assert.equal(options.cache, 'no-store');
    return response(url.includes('/runs') ? [run(3, 0, { status: 'error', pons_error: 1 }), run(2, 4), run(1, 10)] : []);
  };
  const result = await latestInventory();
  assert.equal(result.run.id, 3);
  assert.match(calls[1], /run=3&limit=2000/);
  assert.match(inventoryNotice(result.run, result.rows), /Incomplete/);
});

test('no finished run means no invented current inventory from historical samples', async () => {
  let calls = 0;
  globalThis.fetch = async () => { calls++; return response([run(1, 0, { status: 'running', finished_at: undefined })]); };
  assert.deepEqual(await latestInventory(), { rows: [] });
  assert.equal(calls, 1);
});

test('truncated and capped snapshots are visibly incomplete', () => {
  assert.match(inventoryNotice(run(1, 3), [{}]), /Incomplete/);
  assert.match(inventoryNotice(run(1, 2000), Array(2000).fill({})), /limited/);
});

test('unknown discovery does not become a zero count', () => {
  for (const status of ['unsupported', 'unavailable', 'partial']) assert.equal(discoveryCount({ status, count: 0 }), '—');
  assert.equal(discoveryCount({ status: 'empty', count: 0 }), 0);
});

test('live read rejects an ONU replaced at the same position or missing serial', async () => {
  for (const serial_number of ['TEST00000002', '']) {
    globalThis.fetch = async () => response({ board: 2, pon: 1, onu_id: 4, serial_number });
    await assert.rejects(liveOnu(2, 1, 4, 'TEST00000001'), /identity could not be confirmed/);
  }
});

test('confirmed live identity is returned; cancellation reaches fetch', async () => {
  const signal = new AbortController().signal;
  globalThis.fetch = async (_url, opts) => {
    assert.equal(opts.signal, signal);
    return response({ board: 2, pon: 1, onu_id: 4, serial_number: 'TEST00000001' });
  };
  assert.equal((await liveOnu(2, 1, 4, 'TEST00000001', signal)).serial_number, 'TEST00000001');
});
