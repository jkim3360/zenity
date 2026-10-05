import { setImmediate } from 'node:timers/promises';
import { log } from './log.js';

// Lane names. Each lane is a bounded queue drained by its own worker.
export const LANE_POST = 'post';
export const LANE_ENGAGEMENT = 'engagement';
export const LANE_FOLLOW = 'follow';
export const LANE_DELETE = 'delete';

/** Returns the lane for an event from its kind, operation and collection, or '' if no lane handles it. */
export function routeOf(e) {
  if (e.kind !== 'commit' || !e.commit) return '';
  if (e.commit.operation === 'delete') return LANE_DELETE;
  if (e.commit.operation !== 'create') return '';
  switch (e.commit.collection) {
    case 'app.bsky.feed.post':
      return LANE_POST;
    case 'app.bsky.feed.like':
    case 'app.bsky.feed.repost':
      return LANE_ENGAGEMENT;
    case 'app.bsky.graph.follow':
      return LANE_FOLLOW;
    default:
      return '';
  }
}

// Lane is one bounded queue, the handler its worker runs, and its counters.
class Lane {
  /** Creates an empty lane and starts its worker. */
  constructor(name, handle, queueSize) {
    this.name = name;
    this.handle = handle;
    this.queueSize = queueSize;
    this.queue = [];
    this.processed = 0;
    this.dropped = 0;
    this.closed = false;
    this.wake = null;
    this.done = this.work();
  }

  /** Queues an event, or drops and counts it if the queue is full. */
  push(e) {
    if (this.queue.length >= this.queueSize) {
      this.dropped++;
      return;
    }
    this.queue.push(e);
    this.wake?.();
  }

  /** Runs the handler on each queued event, one at a time, until the lane is closed and empty. */
  async work() {
    for (;;) {
      if (this.queue.length === 0) {
        if (this.closed) return;
        await new Promise((resolve) => {
          this.wake = resolve;
        });
        this.wake = null;
        continue;
      }
      await this.run(this.queue.shift());
      await setImmediate(); // give the socket and the other lanes a turn
    }
  }

  /** Handles one event and counts it, catching errors so one bad event cannot stop the lane. */
  async run(e) {
    try {
      await this.handle(e);
    } catch (err) {
      log('error', 'handler failed', { lane: this.name, error: String(err) });
    } finally {
      this.processed++;
    }
  }

  /** Marks the lane closed so its worker returns once the queue is empty. */
  close() {
    this.closed = true;
    this.wake?.();
  }
}

// Dispatcher fans events out to one bounded lane per event type.
export class Dispatcher {
  /** Starts one lane per handler, each with its own queue of queueSize events. */
  constructor(handlers, queueSize) {
    this.lanes = new Map(Object.entries(handlers).map(([name, handle]) => [name, new Lane(name, handle, queueSize)]));
  }

  /** Routes an event to its lane without blocking, dropping and counting it if that lane is full. */
  dispatch(e) {
    this.lanes.get(routeOf(e))?.push(e);
  }

  /** Logs each lane's queue depth and its processed and dropped counts. */
  logStats() {
    for (const name of [...this.lanes.keys()].sort()) {
      const l = this.lanes.get(name);
      log('info', 'lane stats', { lane: name, queued: l.queue.length, processed: l.processed, dropped: l.dropped });
    }
  }

  /** Stops the lanes from accepting events and waits for each to drain its backlog. */
  async close() {
    for (const l of this.lanes.values()) l.close();
    await Promise.all([...this.lanes.values()].map((l) => l.done));
  }
}
