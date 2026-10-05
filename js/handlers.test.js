import assert from 'node:assert/strict';
import test from 'node:test';
import { matchKeyword, parseKeywords, WindowCounter } from './handlers.js';

// Checks that a key alerts once at the threshold, keys count separately, and the window resets.
test('WindowCounter', () => {
  const c = new WindowCounter(60, 3);
  const t0 = 1_700_000_000_000_000;
  const s = 1_000_000;
  const steps = [
    ['a', 0, false],
    ['a', 1 * s, false],
    ['b', 2 * s, false],
    ['a', 3 * s, true], // third hit on a
    ['a', 4 * s, false], // fires once per window
    ['a', 61 * s, false],
    ['a', 62 * s, false],
    ['a', 63 * s, true], // new window, counting from zero
  ];
  steps.forEach(([key, at, want], i) => assert.equal(c.hit(key, t0 + at), want, `step ${i}`));
});

// Checks case-insensitive matching and misses.
test('matchKeyword', () => {
  const keywords = ['golang', 'kubernetes'];
  const cases = {
    'I love GoLang': 'golang',
    'KUBERNETES at 3am': 'kubernetes',
    'hello world': '',
    '': '',
    'go lang is two words': '',
  };
  for (const [text, want] of Object.entries(cases)) assert.equal(matchKeyword(text, keywords), want, text);
});

// Checks trimming, lowercasing and dropping blanks.
test('parseKeywords', () => {
  assert.deepEqual(parseKeywords(' Go , ,KUBERNETES,,'), ['go', 'kubernetes']);
  assert.deepEqual(parseKeywords(' , '), []);
});
