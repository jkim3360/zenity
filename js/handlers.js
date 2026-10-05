import { log } from './log.js';
import { LANE_DELETE, LANE_ENGAGEMENT, LANE_FOLLOW, LANE_POST } from './router.js';

// WindowCounter counts hits per key in a tumbling window. Only its own lane's worker uses it.
export class WindowCounter {
  /** Creates a counter that reports a key once per window, when it reaches threshold hits. */
  constructor(windowSeconds, threshold) {
    this.windowSeconds = windowSeconds;
    this.threshold = threshold;
    this.start = -Infinity;
    this.counts = new Map();
  }

  /** Records one event for key at event time timeUS and reports whether the key just reached the threshold. */
  hit(key, timeUS) {
    if (timeUS - this.start >= this.windowSeconds * 1e6) {
      this.start = timeUS;
      this.counts.clear();
    }
    const count = (this.counts.get(key) ?? 0) + 1;
    this.counts.set(key, count);
    return count === this.threshold;
  }
}

/** Splits a comma-separated list into lowercase keywords, dropping blanks. */
export function parseKeywords(s) {
  return s
    .split(',')
    .map((k) => k.trim().toLowerCase())
    .filter(Boolean);
}

/** Returns the first keyword that appears in text, ignoring case, or '' if none does. */
export function matchKeyword(text, keywords) {
  const lower = text.toLowerCase();
  return keywords.find((k) => lower.includes(k)) ?? '';
}

/** Returns a handler that raises a notification when a new post mentions a keyword, without logging the text. */
export function postHandler(keywords) {
  return (e) => {
    const text = e.commit.record?.text;
    if (typeof text !== 'string') return;
    const keyword = matchKeyword(text, keywords);
    if (keyword) log('info', 'keyword notification', { lane: LANE_POST, keyword, did: e.did, rkey: e.commit.rkey });
  };
}

/** Returns a handler that alerts when one post's likes and reposts reach the threshold within a window. */
export function engagementHandler(c) {
  return (e) => {
    const subject = e.commit.record?.subject?.uri;
    if (typeof subject !== 'string' || !subject) return;
    if (c.hit(subject, e.time_us)) {
      log('info', 'engagement alert', { lane: LANE_ENGAGEMENT, subject, count: c.threshold, window_seconds: c.windowSeconds });
    }
  };
}

/** Returns a handler that alerts when one account gains the threshold of follows within a window. */
export function followHandler(c) {
  return (e) => {
    const account = e.commit.record?.subject;
    if (typeof account !== 'string' || !account) return;
    if (c.hit(account, e.time_us)) {
      log('info', 'follow burst', { lane: LANE_FOLLOW, account, count: c.threshold, window_seconds: c.windowSeconds });
    }
  };
}

/** Logs each retraction at debug level, standing in for a cleanup pipeline. */
export function deleteHandler(e) {
  log('debug', 'retraction', { lane: LANE_DELETE, collection: e.commit.collection, did: e.did, rkey: e.commit.rkey });
}
