import { setTimeout as sleep } from 'node:timers/promises';
import WebSocket from 'ws';
import { log } from './log.js';

// The wantedCollections filter: Jetstream sends only these record types (plus identity and account events).
export const COLLECTIONS = ['app.bsky.feed.post', 'app.bsky.feed.like', 'app.bsky.feed.repost', 'app.bsky.graph.follow'];

const RECONNECT_DELAY_MS = 2000;
const RESUME_REWIND_US = 5_000_000; // replay a little on resume, so a reconnect loses nothing (at-least-once)

/** Reads the stream until signal aborts, reconnecting from the last seen time_us after any error. */
export async function consume(signal, base, dispatch) {
  let last = 0;
  while (!signal.aborted) {
    const err = await readStream(signal, streamURL(base, last), (e) => {
      last = e.time_us;
      dispatch(e);
    });
    if (signal.aborted) return;
    log('warn', 'disconnected', { error: err.message, retry_in: `${RECONNECT_DELAY_MS / 1000}s` });
    await sleep(RECONNECT_DELAY_MS, undefined, { signal }).catch(() => {});
  }
}

/** Builds the subscribe URL with the collection filter and, once an event has been seen, a rewound cursor. */
export function streamURL(base, last) {
  const q = new URLSearchParams();
  if (last > 0) q.set('cursor', String(last - RESUME_REWIND_US));
  for (const c of COLLECTIONS) q.append('wantedCollections', c);
  return `${base}?${q}`;
}

/** Connects to url and passes each decoded event to handle, resolving with the error that ended the connection. */
function readStream(signal, url, handle) {
  return new Promise((resolve) => {
    const ws = new WebSocket(url, { maxPayload: 1 << 20 });
    const stop = () => ws.terminate();
    let error;
    signal.addEventListener('abort', stop, { once: true });
    ws.on('open', () => log('info', 'connected', { url }));
    ws.on('message', (data) => {
      let e;
      try {
        e = JSON.parse(data);
      } catch {
        log('warn', 'undecodable event', { bytes: data.length });
        return;
      }
      handle(e);
    });
    ws.on('error', (err) => {
      error = err;
    });
    ws.on('close', (code) => {
      signal.removeEventListener('abort', stop);
      resolve(error ?? new Error(`connection closed with code ${code}`));
    });
  });
}
