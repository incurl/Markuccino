/**
 * Pure binder logic.
 *
 * The backend sends a flat list of items; the tree, the visible rows and
 * the drag-and-drop indexes are computed here. Keeping it out of the
 * component is what makes it testable.
 */

import type { BinderItem } from '../../bindings';

/** One node of the binder tree. */
export interface BinderNode {
  item: BinderItem;
  children: BinderNode[];
  depth: number;
}

/** One rendered row of the binder. */
export interface BinderRow {
  item: BinderItem;
  depth: number;
  hasChildren: boolean;
  expanded: boolean;
}

/**
 * buildTree assembles the flat list into a tree, ordered by position.
 *
 * Items whose parent is missing (or that point at themselves) are
 * promoted to the root: a binder must never hide a chapter because one
 * record is inconsistent.
 */
export function buildTree(items: BinderItem[]): BinderNode[] {
  const byParent = new Map<string, BinderItem[]>();
  const ids = new Set(items.map((item) => item.id));

  for (const item of items) {
    const parent = item.parent_id && item.parent_id !== item.id && ids.has(item.parent_id) ? item.parent_id : '';
    const list = byParent.get(parent);
    if (list) {
      list.push(item);
    } else {
      byParent.set(parent, [item]);
    }
  }

  const build = (parent: string, depth: number, seen: Set<string>): BinderNode[] => {
    const children = (byParent.get(parent) ?? []).slice().sort(compareItems);
    const nodes: BinderNode[] = [];
    for (const item of children) {
      if (seen.has(item.id)) {
        continue;
      }
      seen.add(item.id);
      nodes.push({ item, children: build(item.id, depth + 1, seen), depth });
    }
    return nodes;
  };

  return build('', 0, new Set());
}

/**
 * flattenTree returns the visible rows, descending only into expanded
 * folders.
 */
export function flattenTree(nodes: BinderNode[], expanded: Set<string>): BinderRow[] {
  const rows: BinderRow[] = [];
  const walk = (list: BinderNode[]) => {
    for (const node of list) {
      const isExpanded = expanded.has(node.item.id);
      rows.push({
        item: node.item,
        depth: node.depth,
        hasChildren: node.children.length > 0,
        expanded: isExpanded,
      });
      if (isExpanded && node.children.length > 0) {
        walk(node.children);
      }
    }
  };
  walk(nodes);
  return rows;
}

/** compareItems orders siblings by position, then by creation. */
export function compareItems(a: BinderItem, b: BinderItem): number {
  if (a.position !== b.position) {
    return a.position - b.position;
  }
  return a.created.localeCompare(b.created);
}

/**
 * descendantIDs returns the ids below rootID, in document order.
 *
 * It is what a Scrivenings view of a folder shows: every writable
 * section below it, in the order the draft reads.
 */
export function descendantIDs(items: BinderItem[], rootID: string): string[] {
  const children = new Map<string, BinderItem[]>();
  for (const item of items) {
    const list = children.get(item.parent_id);
    if (list) {
      list.push(item);
    } else {
      children.set(item.parent_id, [item]);
    }
  }

  const result: string[] = [];
  const walk = (id: string, seen: Set<string>) => {
    for (const child of (children.get(id) ?? []).slice().sort(compareItems)) {
      if (seen.has(child.id)) {
        continue;
      }
      seen.add(child.id);
      if (child.kind === 'document') {
        result.push(child.id);
      }
      walk(child.id, seen);
    }
  };
  walk(rootID, new Set());
  return result;
}

/**
 * matchingIDs returns the items that match a query plus their ancestors,
 * so a filtered tree still shows where a match lives.
 *
 * An empty query matches everything.
 */
export function matchingIDs(items: BinderItem[], query: string): Set<string> {
  const needle = query.trim().toLowerCase();
  if (!needle) {
    return new Set(items.map((item) => item.id));
  }

  const byID = new Map(items.map((item) => [item.id, item]));
  const matches = new Set<string>();
  for (const item of items) {
    const haystack = `${item.title} ${item.synopsis} ${item.keywords.join(' ')}`.toLowerCase();
    if (!haystack.includes(needle)) {
      continue;
    }
    matches.add(item.id);
    let parent = item.parent_id;
    const seen = new Set<string>();
    while (parent && !seen.has(parent)) {
      seen.add(parent);
      matches.add(parent);
      parent = byID.get(parent)?.parent_id ?? '';
    }
  }
  return matches;
}

/**
 * buildRows is the binder's one entry point: it builds the tree, applies
 * the filter and expands the folders the filter needs.
 */
export function buildRows(items: BinderItem[], query: string, expanded: Set<string>): BinderRow[] {
  const tree = buildTree(items);
  const matching = matchingIDs(items, query);
  const filtered = query.trim() ? filterTree(tree, matching) : tree;
  const effectiveExpanded = query.trim() ? new Set(items.map((item) => item.id)) : expanded;
  return flattenTree(filtered, effectiveExpanded);
}

/** filterTree keeps the matching nodes and the ancestors of a match. */
function filterTree(nodes: BinderNode[], matching: Set<string>): BinderNode[] {
  const result: BinderNode[] = [];
  for (const node of nodes) {
    const children = filterTree(node.children, matching);
    if (matching.has(node.item.id) || children.length > 0) {
      result.push({ ...node, children });
    }
  }
  return result;
}

/**
 * dropTarget computes where a dropped item goes.
 *
 * Dropping on a folder puts the item inside it; dropping on a document
 * puts it beside that document, in the same folder. Dropping on a
 * document that is the dragged item's sibling reorders it.
 */
export function dropTarget(
  items: BinderItem[],
  draggedID: string,
  targetID: string,
): { parentID: string; index: number } | null {
  if (draggedID === targetID) {
    return null;
  }
  const target = items.find((item) => item.id === targetID);
  const dragged = items.find((item) => item.id === draggedID);
  if (!target || !dragged) {
    return null;
  }

  if (target.kind === 'folder') {
    // A folder cannot be dropped into its own subtree: that would
    // detach the subtree from the tree.
    if (isDescendant(items, targetID, draggedID)) {
      return null;
    }
    const siblings = items.filter((item) => item.parent_id === targetID && item.id !== draggedID);
    return { parentID: targetID, index: siblings.length };
  }

  if (isDescendant(items, target.parent_id, draggedID)) {
    return null;
  }

  const siblings = items
    .filter((item) => item.parent_id === target.parent_id && item.id !== draggedID)
    .sort(compareItems);
  const index = siblings.findIndex((item) => item.id === targetID);
  return { parentID: target.parent_id, index: index < 0 ? siblings.length : index };
}

/** isDescendant reports whether candidate sits below ancestor. */
export function isDescendant(items: BinderItem[], candidate: string, ancestor: string): boolean {
  const byID = new Map(items.map((item) => [item.id, item]));
  let current = candidate;
  const seen = new Set<string>();
  while (current && !seen.has(current)) {
    if (current === ancestor) {
      return true;
    }
    seen.add(current);
    current = byID.get(current)?.parent_id ?? '';
  }
  return false;
}

/** findRoot returns the top-level group of a bucket. */
export function findRoot(items: BinderItem[], bucket: string): BinderItem | undefined {
  return items.find((item) => item.parent_id === '' && item.bucket === bucket);
}

/** isRoot reports whether an item is a top-level group. */
export function isRoot(item: BinderItem): boolean {
  return item.parent_id === '';
}
