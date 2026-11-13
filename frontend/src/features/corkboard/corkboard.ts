/**
 * Pure corkboard logic: which cards a selection shows, how the cards are
 * ordered and where a dragged card lands.
 */

import type { BinderItem } from '../../bindings';
import { compareItems, dropTarget } from '../binder/binder';

/** A card on the board. */
export interface Card {
  item: BinderItem;
  /** The number of writable sections directly below a folder card. */
  children: number;
}

/**
 * cardsFor returns the index cards the board shows.
 *
 * A folder shows its direct children; a document shows its siblings, so
 * opening a card's board keeps the surrounding scene in view. The three
 * top-level groups are never turned into cards.
 */
export function cardsFor(items: BinderItem[], selectedId: string): Card[] {
  const selected = items.find((item) => item.id === selectedId);
  if (!selected) {
    return [];
  }
  const parentId = selected.kind === 'folder' ? selected.id : selected.parent_id;
  if (!parentId) {
    return [];
  }

  const children = items.filter((item) => item.parent_id === parentId).sort(compareItems);
  return children.map((item) => ({
    item,
    children: items.filter((child) => child.parent_id === item.id).length,
  }));
}

/** cardsWords sums the words of the cards that hold a body. */
export function cardsWords(cards: Card[]): number {
  return cards.reduce((total, card) => total + (card.item.kind === 'document' ? card.item.word_count : 0), 0);
}

/**
 * cardDrop computes the move a card drop performs.
 *
 * Dropping one card onto another inserts it before that card; dropping
 * it onto the board's empty area is the caller's job (it appends).
 */
export function cardDrop(
  items: BinderItem[],
  draggedId: string,
  targetId: string,
): { parentID: string; index: number } | null {
  return dropTarget(items, draggedId, targetId);
}

/** folderTitle names the board. */
export function folderTitle(items: BinderItem[], selectedId: string): string {
  const selected = items.find((item) => item.id === selectedId);
  if (!selected) {
    return 'Corkboard';
  }
  if (selected.kind === 'folder') {
    return selected.title;
  }
  const parent = items.find((item) => item.id === selected.parent_id);
  return parent ? parent.title : selected.title;
}
