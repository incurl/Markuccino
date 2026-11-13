import { describe, expect, it } from 'vitest';

import type { BinderItem } from '../../bindings';
import {
  buildRows,
  buildTree,
  descendantIDs,
  dropTarget,
  findRoot,
  flattenTree,
  isDescendant,
  matchingIDs,
} from './binder';

/** item builds a binder item with sensible defaults. */
function item(overrides: Partial<BinderItem> & Pick<BinderItem, 'id' | 'title'>): BinderItem {
  return {
    project_id: 'p1',
    parent_id: '',
    kind: 'document',
    bucket: 'draft',
    file_path: `documents/${overrides.id}.md`,
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

const draft = item({ id: 'draft', title: 'Manuscript', kind: 'folder', position: 0 });
const research = item({ id: 'research', title: 'Research', kind: 'folder', position: 1 });
const trash = item({ id: 'trash', title: 'Trash', kind: 'folder', position: 2, bucket: 'trash' });

const partOne = item({ id: 'part1', title: 'Part One', kind: 'folder', parent_id: 'draft', position: 0 });
const chapterOne = item({ id: 'ch1', title: 'Chapter One', parent_id: 'part1', position: 0, word_count: 100 });
const chapterTwo = item({ id: 'ch2', title: 'Chapter Two', parent_id: 'part1', position: 1, word_count: 200 });
const scene = item({ id: 'scene', title: 'Loose Scene', parent_id: 'draft', position: 1, word_count: 50 });

const all = [draft, research, trash, partOne, chapterOne, chapterTwo, scene];

describe('buildTree', () => {
  it('orders the roots and the siblings by position', () => {
    const tree = buildTree(all);
    expect(tree.map((node) => node.item.id)).toEqual(['draft', 'research', 'trash']);
    expect(tree[0].children.map((node) => node.item.id)).toEqual(['part1', 'scene']);
    expect(tree[0].children[0].children.map((node) => node.item.id)).toEqual(['ch1', 'ch2']);
  });

  it('promotes an orphan to the root instead of hiding it', () => {
    const orphan = item({ id: 'orphan', title: 'Orphan', parent_id: 'missing' });
    const tree = buildTree([draft, orphan]);
    expect(tree.map((node) => node.item.id)).toContain('orphan');
  });

  it('survives a parent cycle', () => {
    const a = item({ id: 'a', title: 'A', kind: 'folder', parent_id: 'b' });
    const b = item({ id: 'b', title: 'B', kind: 'folder', parent_id: 'a' });
    expect(() => buildTree([a, b])).not.toThrow();
  });
});

describe('flattenTree', () => {
  it('shows only the expanded branches', () => {
    const rows = flattenTree(buildTree(all), new Set(['draft']));
    expect(rows.map((row) => row.item.id)).toEqual(['draft', 'part1', 'scene', 'research', 'trash']);
    expect(rows[1].depth).toBe(1);
    expect(rows[1].hasChildren).toBe(true);
  });
});

describe('descendantIDs', () => {
  it('returns the documents below a folder in reading order', () => {
    expect(descendantIDs(all, 'draft')).toEqual(['ch1', 'ch2', 'scene']);
  });

  it('returns nothing for a document', () => {
    expect(descendantIDs(all, 'ch1')).toEqual([]);
  });
});

describe('matchingIDs', () => {
  it('keeps the ancestors of a match', () => {
    const matches = matchingIDs(all, 'chapter two');
    expect(matches.has('ch2')).toBe(true);
    expect(matches.has('part1')).toBe(true);
    expect(matches.has('draft')).toBe(true);
    expect(matches.has('scene')).toBe(false);
  });

  it('matches an empty query against everything', () => {
    expect(matchingIDs(all, '  ').size).toBe(all.length);
  });
});

describe('buildRows', () => {
  it('expands the matching branches while filtering', () => {
    const rows = buildRows(all, 'chapter one', new Set());
    const ids = rows.map((row) => row.item.id);
    expect(ids).toContain('ch1');
    expect(ids).toContain('part1');
    expect(ids).not.toContain('scene');
  });
});

describe('dropTarget', () => {
  it('drops into a folder at the end', () => {
    expect(dropTarget(all, 'scene', 'part1')).toEqual({ parentID: 'part1', index: 2 });
  });

  it('drops beside a document at its index', () => {
    expect(dropTarget(all, 'scene', 'ch2')).toEqual({ parentID: 'part1', index: 1 });
  });

  it('refuses a drop onto itself', () => {
    expect(dropTarget(all, 'ch1', 'ch1')).toBeNull();
  });

  it('refuses a folder dropped into its own subtree', () => {
    expect(dropTarget(all, 'part1', 'ch1')).toBeNull();
    expect(isDescendant(all, 'ch1', 'part1')).toBe(true);
  });
});

describe('findRoot', () => {
  it('finds the top-level group of a bucket', () => {
    expect(findRoot(all, 'draft')?.id).toBe('draft');
    expect(findRoot(all, 'trash')?.id).toBe('trash');
    expect(findRoot(all, 'nope')).toBeUndefined();
  });
});
