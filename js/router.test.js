import assert from 'node:assert/strict';
import test from 'node:test';
import { Dispatcher, LANE_DELETE, LANE_ENGAGEMENT, LANE_FOLLOW, LANE_POST, routeOf } from './router.js';

/** Builds a commit event for the given operation and collection. */
function commit(operation, collection) {
  return { kind: 'commit', commit: { operation, collection } };
}

// Checks that each event type reaches its lane and everything else is ignored.
test('routeOf', async (t) => {
  const cases = [
    ['post create', commit('create', 'app.bsky.feed.post'), LANE_POST],
    ['like create', commit('create', 'app.bsky.feed.like'), LANE_ENGAGEMENT],
    ['repost create', commit('create', 'app.bsky.feed.repost'), LANE_ENGAGEMENT],
    ['follow create', commit('create', 'app.bsky.graph.follow'), LANE_FOLLOW],
    ['any delete', commit('delete', 'app.bsky.feed.like'), LANE_DELETE],
    ['post update', commit('update', 'app.bsky.feed.post'), ''],
    ['block create', commit('create', 'app.bsky.graph.block'), ''],
    ['identity event', { kind: 'identity' }, ''],
  ];
  for (const [name, event, want] of cases) {
    await t.test(name, () => assert.equal(routeOf(event), want));
  }
});

// Checks that a stuck lane drops its own overflow while other lanes keep working (the test fails after 1 s).
test('dispatch isolates a slow lane', { timeout: 1000 }, async () => {
  let release;
  const stuck = new Promise((resolve) => (release = resolve));
  let followed;
  const followHandled = new Promise((resolve) => (followed = resolve));
  const d = new Dispatcher({ [LANE_POST]: () => stuck, [LANE_FOLLOW]: () => followed() }, 1);

  for (let i = 0; i < 5; i++) d.dispatch(commit('create', 'app.bsky.feed.post'));
  d.dispatch(commit('create', 'app.bsky.graph.follow'));

  await followHandled;
  assert.ok(d.lanes.get(LANE_POST).dropped >= 3, `post lane dropped ${d.lanes.get(LANE_POST).dropped}, want at least 3`);
  release();
  await d.close();
});

// Checks that a lane keeps processing after its handler throws.
test('dispatch recovers from a failing handler', async (t) => {
  t.mock.method(console, 'log', () => {});
  let calls = 0;
  const d = new Dispatcher(
    {
      [LANE_DELETE]: () => {
        if (++calls === 1) throw new Error('boom');
      },
    },
    10,
  );
  d.dispatch(commit('delete', 'app.bsky.feed.post'));
  d.dispatch(commit('delete', 'app.bsky.feed.post'));
  await d.close();

  assert.equal(d.lanes.get(LANE_DELETE).processed, 2);
});
