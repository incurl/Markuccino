import { describe, expect, it } from 'vitest';

import type { BinderItem } from '../../bindings';
import { outlineRoot, outlineRows, sortRows, targetProgress, totalWords } from './outliner';

function item(overrides: Partial<BinderItem> & Pick<BinderItem, 'id' | 'title'>): BinderItem {
  return {
    project_id: 'p1',
    parent_id: '',
    kind: 'document',
    bucket: 'draft',
    file_path: '',
    position: 0,
    synopsis: '',
    notes: '',
    keywords: [],
    label: '',
    status: '',
    include_in_compile: true,
    word_count: 0,
    char_count: 0,
    custom_meta: {},
    last_commit: '',
    trashed: false,
    created: '2026-01-01T00:00:00Z',
    updated: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

const draft = item({ id: 'draft', title: 'Manuscript', kind: 'folder' });
const part = item({ id: 'part', title: 'Part Two', kind: 'folder', parent_id: 'draft', position: 0 });
const ch1 = item({ id: 'ch1', title: 'Alpha', parent_id: 'part', position: 0, word_count: 300, label: 'Chapter' });
const ch2 = item({ id: 'ch2', title: 'Beta', parent_id: 'part', position: 1, word_count: 100, label: 'Scene' });
const all = [draft, part, ch1, ch2];

describe('outlineRoot', () => {
  it('uses a folder as its own root', () => {
    expect(outlineRoot(all, 'part')).toBe('part');
  });

  it('uses the parent of a document', () => {
    expect(outlineRoot(all, 'ch1')).toBe('part');
  });

  it('falls back to the draft root', () => {
    expect(outlineRoot(all, 'missing')).toBe('draft');
  });
});

describe('outlineRows', () => {
  it('returns the subtree with its depths', () => {
    const rows = outlineRows(all, 'part');
    expect(rows.map((row) => [row.item.id, row.depth])).toEqual([
      ['ch1', 0],
      ['ch2', 0],
    ]);
  });

  it('includes nested folders', () => {
    const rows = outlineRows(all, 'draft');
    expect(rows.map((row) => row.item.id)).toEqual(['part', 'ch1', 'ch2']);
    expect(rows[1].depth).toBe(1);
  });
});

describe('sortRows', () => {
  it('sorts by a column ascending and descending', () => {
    const rows = outlineRows(all, 'part');
    expect(sortRows(rows, 'title', 'asc').map((row) => row.item.id)).toEqual(['ch1', 'ch2']);
    expect(sortRows(rows, 'title', 'desc').map((row) => row.item.id)).toEqual(['ch2', 'ch1']);
  });

  it('sorts by the word count', () => {
    const rows = outlineRows(all, 'part');
    expect(sortRows(rows, 'words', 'asc').map((row) => row.item.id)).toEqual(['ch2', 'ch1']);
  });
});

describe('totalWords and targetProgress', () => {
  it('sums only the documents', () => {
    expect(totalWords(outlineRows(all, 'draft'))).toBe(400);
  });

  it('clamps the progress to a percentage', () => {
    expect(targetProgress(250, 1000)).toBe(25);
    expect(targetProgress(2000, 1000)).toBe(100);
    expect(targetProgress(500, 0)).toBe(0);
  });
});
