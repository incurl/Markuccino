/**
 * Pure editor logic: the formatting commands, the counters and the
 * find/replace helpers. The textarea itself is the framework's job.
 */

/** The editor's text and selection, as a formatting command sees it. */
export interface TextState {
  text: string;
  selectionStart: number;
  selectionEnd: number;
}

/** The formatting commands the toolbar offers. */
export type FormatAction =
  | 'bold'
  | 'italic'
  | 'code'
  | 'h1'
  | 'h2'
  | 'quote'
  | 'bullet'
  | 'numbered';

/** Counts the words of the editor buffer. */
export function countWords(text: string): number {
  const words = text.trim().split(/\s+/);
  return words.length === 1 && words[0] === '' ? 0 : words.length;
}

/** Counts the characters, optionally including the whitespace. */
export function countCharacters(text: string, withSpaces = false): number {
  if (withSpaces) {
    return Array.from(text).length;
  }
  return Array.from(text.replace(/\s/g, '')).length;
}

/** Estimates the reading time in minutes, at 220 words a minute. */
export function readingMinutes(words: number): number {
  return Math.max(1, Math.round(words / 220));
}

/**
 * applyFormat applies a formatting command to a selection.
 *
 * Inline commands wrap the selection; line commands toggle a prefix on
 * every line the selection touches. Applying the same command twice
 * removes it, which is what a toggle in a toolbar is expected to do.
 */
export function applyFormat(state: TextState, action: FormatAction): TextState {
  switch (action) {
    case 'bold':
      return wrapSelection(state, '**', '**');
    case 'italic':
      return wrapSelection(state, '_', '_');
    case 'code':
      return wrapSelection(state, '`', '`');
    case 'h1':
      return toggleLinePrefix(state, '# ');
    case 'h2':
      return toggleLinePrefix(state, '## ');
    case 'quote':
      return toggleLinePrefix(state, '> ');
    case 'bullet':
      return toggleLinePrefix(state, '- ');
    case 'numbered':
      return toggleLinePrefix(state, '1. ');
    default:
      return state;
  }
}

/** wrapSelection surrounds the selection with a marker. */
function wrapSelection(state: TextState, prefix: string, suffix: string): TextState {
  const { text, selectionStart, selectionEnd } = state;
  const selected = text.slice(selectionStart, selectionEnd);

  // A second application removes the markers instead of nesting them.
  const before = text.slice(Math.max(0, selectionStart - prefix.length), selectionStart);
  const after = text.slice(selectionEnd, selectionEnd + suffix.length);
  if (before === prefix && after === suffix) {
    return {
      text:
        text.slice(0, selectionStart - prefix.length) +
        selected +
        text.slice(selectionEnd + suffix.length),
      selectionStart: selectionStart - prefix.length,
      selectionEnd: selectionEnd - prefix.length,
    };
  }

  return {
    text: text.slice(0, selectionStart) + prefix + selected + suffix + text.slice(selectionEnd),
    selectionStart: selectionStart + prefix.length,
    selectionEnd: selectionEnd + prefix.length,
  };
}

/** toggleLinePrefix adds or removes a prefix on every selected line. */
function toggleLinePrefix(state: TextState, prefix: string): TextState {
  const { text, selectionStart, selectionEnd } = state;
  const lineStart = text.lastIndexOf('\n', selectionStart - 1) + 1;
  const nextBreak = text.indexOf('\n', selectionEnd);
  const lineEnd = nextBreak === -1 ? text.length : nextBreak;
  const block = text.slice(lineStart, lineEnd);
  const lines = block.split('\n');

  const allPrefixed = lines.every((line) => line.startsWith(prefix));
  const replacement = lines
    .map((line) => (allPrefixed ? line.slice(prefix.length) : prefix + line))
    .join('\n');

  const delta = replacement.length - block.length;
  return {
    text: text.slice(0, lineStart) + replacement + text.slice(lineEnd),
    selectionStart: lineStart,
    selectionEnd: Math.max(lineStart, selectionEnd + delta),
  };
}

/**
 * findMatches returns the starting indexes of every case-insensitive
 * occurrence of query.
 */
export function findMatches(text: string, query: string): number[] {
  if (!query) {
    return [];
  }
  const haystack = text.toLowerCase();
  const needle = query.toLowerCase();
  const result: number[] = [];
  let index = haystack.indexOf(needle);
  while (index !== -1) {
    result.push(index);
    index = haystack.indexOf(needle, index + needle.length);
  }
  return result;
}

/** replaceAllText replaces every case-insensitive occurrence. */
export function replaceAllText(text: string, query: string, replacement: string): string {
  if (!query) {
    return text;
  }
  const pattern = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return text.replace(new RegExp(pattern, 'gi'), replacement);
}

/**
 * sectionSeparator is the line the Scrivenings view draws between two
 * sections. It is presentation only: the stored bodies never contain it.
 */
export function sectionSeparator(title: string): string {
  return `\n\n<!-- ${title} -->\n\n`;
}
