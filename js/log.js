// A tiny stand-in for Go's slog JSON handler: one JSON line per record on stdout, with the same field names.

const LEVELS = { debug: -4, info: 0, warn: 4, error: 8 };
let minLevel = LEVELS.info;

/** Sets the lowest level that gets written and returns false if the name is not a known level. */
export function setLevel(name) {
  const level = LEVELS[name.toLowerCase()];
  if (level === undefined) return false;
  minLevel = level;
  return true;
}

/** Writes one JSON log line with time, level and msg followed by the given fields. */
export function log(level, msg, fields = {}) {
  if (LEVELS[level] < minLevel) return;
  console.log(JSON.stringify({ time: new Date().toISOString(), level: level.toUpperCase(), msg, ...fields }));
}
