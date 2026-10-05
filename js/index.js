// jetstream-router reads the Bluesky Jetstream and routes each event to an independent worker lane by type.
import { deleteHandler, engagementHandler, followHandler, parseKeywords, postHandler, WindowCounter } from './handlers.js';
import { log, setLevel } from './log.js';
import { Dispatcher, LANE_DELETE, LANE_ENGAGEMENT, LANE_FOLLOW, LANE_POST } from './router.js';
import { consume } from './stream.js';

/** Returns the named environment variable, or def if it is unset or empty. */
function env(name, def) {
  return process.env[name] || def;
}

/** Reads a positive integer from the environment and exits if the value is not one. */
function envInt(name, def) {
  const v = env(name, String(def));
  const n = Number(v);
  if (!/^\d+$/.test(v) || n <= 0) fatal(`${name} must be a positive integer`, { value: v });
  return n;
}

/** Logs an error and exits with status 1. */
function fatal(msg, fields) {
  log('error', msg, fields);
  process.exit(1);
}

/** Logs lane stats every ms milliseconds until signal aborts. */
function logStatsEvery(signal, d, ms) {
  const timer = setInterval(() => d.logStats(), ms);
  signal.addEventListener('abort', () => clearInterval(timer), { once: true });
}

/** Reads config, starts the four lanes and the stream reader, and on SIGINT or SIGTERM drains the lanes and exits. */
async function main() {
  if (!setLevel(env('LOG_LEVEL', 'info'))) {
    fatal('LOG_LEVEL must be debug, info, warn or error', { value: process.env.LOG_LEVEL });
  }
  const keywords = parseKeywords(env('KEYWORDS', 'golang,kubernetes'));
  if (keywords.length === 0) fatal('KEYWORDS must list at least one keyword', { value: process.env.KEYWORDS });
  const engagementThreshold = envInt('ENGAGEMENT_THRESHOLD', 25);
  const followThreshold = envInt('FOLLOW_THRESHOLD', 10);
  const windowSeconds = envInt('WINDOW_SECONDS', 60);
  const queueSize = envInt('QUEUE_SIZE', 1000);
  const jetstreamURL = env('JETSTREAM_URL', 'wss://jetstream2.us-east.bsky.network/subscribe');
  if (!URL.canParse(jetstreamURL) || !['ws:', 'wss:'].includes(new URL(jetstreamURL).protocol)) {
    fatal('JETSTREAM_URL must be a ws:// or wss:// URL', { value: jetstreamURL });
  }
  log('info', 'starting', {
    keywords,
    engagement_threshold: engagementThreshold,
    follow_threshold: followThreshold,
    window_seconds: windowSeconds,
    queue_size: queueSize,
  });

  const d = new Dispatcher(
    {
      [LANE_POST]: postHandler(keywords),
      [LANE_ENGAGEMENT]: engagementHandler(new WindowCounter(windowSeconds, engagementThreshold)),
      [LANE_FOLLOW]: followHandler(new WindowCounter(windowSeconds, followThreshold)),
      [LANE_DELETE]: deleteHandler,
    },
    queueSize,
  );

  const ac = new AbortController();
  for (const sig of ['SIGINT', 'SIGTERM']) process.once(sig, () => ac.abort()); // a second signal kills the process
  logStatsEvery(ac.signal, d, 10_000);
  await consume(ac.signal, jetstreamURL, (e) => d.dispatch(e));
  log('info', 'shutting down');
  await d.close();
  d.logStats();
}

await main();
