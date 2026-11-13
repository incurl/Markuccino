import { describe, expect, it } from 'vitest';

import type { BinderItem } from '../../bindings';
import { cardsFor, cardsWords, cardDrop, folderTitle } from './corkboard';

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
const part = item({ id: 'part', title: 'Part One', kind: 'folder', parent_id: 'draft', position: 0 });
const one = item({ id: 'one', title: 'One', parent_id: 'part', position: 0, word_count: 100 });
const two = item({ id: 'two', title: 'Two', parent_id: 'part', position: 1, word_count: 50 });
const all = [draft, part, one, two];

describe('cardsFor', () => {
  it('shows the children of a folder', () => {
    expect(cardsFor(all, 'part').map((card) => card.item.id)).toEqual(['one', 'two']);
  });

  it('shows the siblings of a document', () => {
    expect(cardsFor(all, 'one').map((card) => card.item.id)).toEqual(['one', 'two']);
  });

  it('shows nothing for an unknown selection', () => {
    expect(cardsFor(all, 'missing')).toEqual([]);
  });

  it('counts the children of a folder card', () => {
    const cards = cardsFor(all, 'draft');
    expect(cards.map((card) => card.item.id)).toEqual(['part']);
    expect(cards[0].children).toBe(2);
  });
});

describe('cardsWords', () => {
  it('sums the words of the documents', () => {
    expect(cardsWords(cardsFor(all, 'part'))).toBe(150);
  });
});

describe('cardDrop', () => {
  it('reorders a card before its target', () => {
    expect(cardDrop(all, 'two', 'one')).toEqual({ parentID: 'part', index: 0 });
  });

  it('refuses a drop onto itself', () => {
    expect(cardDrop(all, 'one', 'one')).toBeNull();
  });
});

describe('folderTitle', () => {
  it('names the board after the folder, or after the parent of a document', () => {
    expect(folderTitle(all, 'part')).toBe('Part One');
    expect(folderTitle(all, 'one')).toBe('Part One');
    expect(folderTitle(all, 'missing')).toBe('Corkboard');
  });
});
