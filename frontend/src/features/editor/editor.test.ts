import { describe, expect, it } from 'vitest';

import {
  applyFormat,
  countCharacters,
  countWords,
  findMatches,
  readingMinutes,
  replaceAllText,
} from './editor';

describe('countWords', () => {
  it('counts the words of a buffer', () => {
    expect(countWords('One two three')).toBe(3);
    expect(countWords('')).toBe(0);
    expect(countWords('   ')).toBe(0);
    expect(countWords('line one\nline two')).toBe(4);
  });
});

describe('countCharacters', () => {
  it('counts the characters without the whitespace by default', () => {
    expect(countCharacters('ab cd\nef')).toBe(6);
  });

  it('counts the characters with the whitespace when asked', () => {
    expect(countCharacters('ab cd\nef', true)).toBe(8);
  });
});

describe('readingMinutes', () => {
  it('never reports less than a minute', () => {
    expect(readingMinutes(10)).toBe(1);
    expect(readingMinutes(660)).toBe(3);
  });
});

describe('applyFormat', () => {
  const state = (text: string, start: number, end: number) => ({
    text,
    selectionStart: start,
    selectionEnd: end,
  });

  it('wraps the selection in bold markers', () => {
    const result = applyFormat(state('a word here', 2, 6), 'bold');
    expect(result.text).toBe('a **word** here');
    expect(result.text.slice(result.selectionStart, result.selectionEnd)).toBe('word');
  });

  it('removes the markers on a second application', () => {
    const once = applyFormat(state('a word here', 2, 6), 'bold');
    const twice = applyFormat(once, 'bold');
    expect(twice.text).toBe('a word here');
  });

  it('toggles a heading on the current line', () => {
    const result = applyFormat(state('Chapter One', 3, 3), 'h1');
    expect(result.text).toBe('# Chapter One');
    expect(applyFormat(result, 'h1').text).toBe('Chapter One');
  });

  it('prefixes every selected line', () => {
    const result = applyFormat(state('one\ntwo', 0, 7), 'bullet');
    expect(result.text).toBe('- one\n- two');
  });

  it('leaves the text alone for an unknown action', () => {
    const original = state('text', 0, 4);
    expect(applyFormat(original, 'unknown' as never)).toEqual(original);
  });
});

describe('findMatches', () => {
  it('finds every case-insensitive occurrence', () => {
    expect(findMatches('The the THE', 'the')).toEqual([0, 4, 8]);
  });

  it('returns nothing for an empty query', () => {
    expect(findMatches('anything', '')).toEqual([]);
  });
});

describe('replaceAllText', () => {
  it('replaces every occurrence, preserving the case of the match', () => {
    expect(replaceAllText('The the THE', 'the', 'a')).toBe('a a a');
  });

  it('treats the query as text, not as a pattern', () => {
    expect(replaceAllText('a.b a.b', 'a.b', 'x')).toBe('x x');
  });
});
