/**
 * Pure outliner logic: the rows of the outline and the column sorting.
 */

import type { BinderItem } from '../../bindings';
import { buildTree, compareItems } from '../binder/binder';
import type { BinderNode } from '../binder/binder';

/** One row of the outline. */
export interface OutlineRow {
  item: BinderItem;
  depth: number;
}

/** The sortable columns of the outline. */
export type OutlineColumn = 'title' | 'synopsis' | 'label' | 'status' | 'words';

/** The sort direction. */
export type SortDirection = 'asc' | 'desc';

/**
 * outlineRoot chooses the subtree the outliner shows.
 *
 * A folder is its own root; a document shows the folder it lives in, so
 * selecting a scene does not collapse the outline around it.
 */
export function outlineRoot(items: BinderItem[], selectedId: string): string {
  const selected = items.find((item) => item.id === selectedId);
  if (!selected) {
    return items.find((item) => item.parent_id === '' && item.bucket === 'draft')?.id ?? '';
  }
  return selected.kind === 'folder' ? selected.id : selected.parent_id;
}

/** outlineRows returns the subtree below a root, parents before children. */
export function outlineRows(items: BinderItem[], rootId: string): OutlineRow[] {
  const tree = buildTree(items);
  const root = findNode(tree, rootId);
  if (!root) {
    return [];
  }

  const rows: OutlineRow[] = [];
  const walk = (nodes: BinderNode[], depth: number) => {
    for (const node of nodes.slice().sort((a, b) => compareItems(a.item, b.item))) {
      rows.push({ item: node.item, depth });
      walk(node.children, depth + 1);
    }
  };
  walk(root.children, 0);
  return rows;
}

/** findNode finds a node by id in a tree. */
function findNode(nodes: BinderNode[], id: string): BinderNode | undefined {
  for (const node of nodes) {
    if (node.item.id === id) {
      return node;
    }
    const found = findNode(node.children, id);
    if (found) {
      return found;
    }
  }
  return undefined;
}

/** sortRows sorts the rows by a column, keeping a stable order. */
export function sortRows(rows: OutlineRow[], column: OutlineColumn, direction: SortDirection): OutlineRow[] {
  const factor = direction === 'asc' ? 1 : -1;
  return rows.slice().sort((a, b) => factor * compareColumn(a.item, b.item, column));
}

/** compareColumn compares two items by a column. */
export function compareColumn(a: BinderItem, b: BinderItem, column: OutlineColumn): number {
  switch (column) {
    case 'words':
      return a.word_count - b.word_count;
    case 'title':
      return a.title.localeCompare(b.title);
    case 'synopsis':
      return a.synopsis.localeCompare(b.synopsis);
    case 'label':
      return a.label.localeCompare(b.label);
    case 'status':
      return a.status.localeCompare(b.status);
    default:
      return 0;
  }
}

/** totalWords sums the words of the document rows. */
export function totalWords(rows: OutlineRow[]): number {
  return rows.reduce((total, row) => total + (row.item.kind === 'document' ? row.item.word_count : 0), 0);
}

/**
 * targetProgress returns how much of the word target the draft has
 * reached, clamped to 0–100. A target of zero means "no goal".
 */
export function targetProgress(words: number, target: number): number {
  if (target <= 0) {
    return 0;
  }
  return Math.min(100, Math.round((words / target) * 100));
}
