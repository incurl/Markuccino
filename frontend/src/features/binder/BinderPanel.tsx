/**
 * The ring binder: a hierarchical list of folders and documents.
 *
 * The panel is presentational. It computes the visible rows and the drop
 * target with the pure helpers in binder.ts and calls back for every
 * change; the backend owns the ordering.
 */

import { useMemo, useState } from 'react';

import type { BinderItem } from '../../bindings';
import { buildRows, dropTarget, findRoot, isRoot } from './binder';

interface BinderPanelProps {
  /** Every item of the project, flat. */
  items: BinderItem[];
  /** The selected item, or an empty string. */
  selectedId: string;
  /** The folders that are open. */
  expanded: Set<string>;
  /** The binder filter query. */
  query: string;
  /** Disables the actions while a mutation is in flight. */
  busy: boolean;
  onQueryChange: (query: string) => void;
  onSelect: (item: BinderItem) => void;
  onToggle: (itemId: string) => void;
  onMove: (itemId: string, parentId: string, index: number) => void;
  onCreate: (parentId: string, kind: 'folder' | 'document') => void;
  onRename: (itemId: string, title: string) => void;
  onDuplicate: (itemId: string) => void;
  onTrash: (itemId: string) => void;
  onRestore: (itemId: string) => void;
  onDelete: (itemId: string) => void;
}

/**
 * The binder panel.
 */
export function BinderPanel({
  items,
  selectedId,
  expanded,
  query,
  busy,
  onQueryChange,
  onSelect,
  onToggle,
  onMove,
  onCreate,
  onRename,
  onDuplicate,
  onTrash,
  onRestore,
  onDelete,
}: BinderPanelProps) {
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [draftTitle, setDraftTitle] = useState('');
  const [draggingId, setDraggingId] = useState<string | null>(null);

  const rows = useMemo(() => buildRows(items, query, expanded), [items, query, expanded]);
  const selected = items.find((item) => item.id === selectedId) ?? null;
  const draftRoot = findRoot(items, 'draft');
  const trashRoot = findRoot(items, 'trash');

  // A new item goes into the selected folder, beside the selected
  // document, or at the end of the draft.
  const creationParentId = selected
    ? selected.kind === 'folder'
      ? selected.id
      : selected.parent_id
    : (draftRoot?.id ?? '');

  const startRename = (item: BinderItem) => {
    if (isRoot(item)) {
      return;
    }
    setRenamingId(item.id);
    setDraftTitle(item.title);
  };

  const commitRename = () => {
    if (renamingId) {
      const title = draftTitle.trim();
      if (title) {
        onRename(renamingId, title);
      }
    }
    setRenamingId(null);
  };

  const inTrash = selected ? selected.bucket === 'trash' : false;

  return (
    <section className="binder" aria-label="Binder">
      <div className="binder-toolbar">
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy}
          title="New document"
          onClick={() => onCreate(creationParentId, 'document')}
        >
          + Doc
        </button>
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy}
          title="New folder"
          onClick={() => onCreate(creationParentId, 'folder')}
        >
          + Folder
        </button>
        <span className="binder-toolbar-spacer" />
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy || !selected || isRoot(selected)}
          title="Duplicate the selected item"
          onClick={() => selected && onDuplicate(selected.id)}
        >
          Copy
        </button>
      </div>

      <div className="binder-actions">
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy || !selected || isRoot(selected) || inTrash}
          title="Move the selected item to the Trash"
          onClick={() => selected && onTrash(selected.id)}
        >
          Trash
        </button>
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy || !selected || !inTrash}
          title="Restore the selected item"
          onClick={() => selected && onRestore(selected.id)}
        >
          Restore
        </button>
        <button
          type="button"
          className="btn btn-sm"
          disabled={busy || !selected || !inTrash || isRoot(selected)}
          title="Delete the selected item permanently"
          onClick={() => selected && onDelete(selected.id)}
        >
          Delete
        </button>
      </div>

      <input
        className="binder-search"
        type="search"
        value={query}
        placeholder="Filter the binder…"
        aria-label="Filter the binder"
        onChange={(event) => onQueryChange(event.target.value)}
      />

      <div className="binder-tree" role="tree">
        {rows.map((row) => {
          const item = row.item;
          const isSelected = item.id === selectedId;
          return (
            <div
              key={item.id}
              role="treeitem"
              aria-selected={isSelected}
              aria-expanded={row.hasChildren ? row.expanded : undefined}
              className={`binder-row${isSelected ? ' is-selected' : ''}${
                item.trashed ? ' is-trashed' : ''
              }`}
              style={{ paddingLeft: `${0.35 + row.depth * 0.9}rem` }}
              draggable={!isRoot(item)}
              onDragStart={() => setDraggingId(item.id)}
              onDragEnd={() => setDraggingId(null)}
              onDragOver={(event) => {
                if (draggingId) {
                  event.preventDefault();
                }
              }}
              onDrop={(event) => {
                event.preventDefault();
                if (!draggingId) {
                  return;
                }
                const target = dropTarget(items, draggingId, item.id);
                if (target) {
                  onMove(draggingId, target.parentID, target.index);
                }
                setDraggingId(null);
              }}
              onClick={() => onSelect(item)}
              onDoubleClick={() => startRename(item)}
            >
              <button
                type="button"
                className="binder-twist"
                aria-label={row.expanded ? 'Collapse the folder' : 'Expand the folder'}
                onClick={(event) => {
                  event.stopPropagation();
                  onToggle(item.id);
                }}
              >
                {row.hasChildren ? (row.expanded ? '▾' : '▸') : '·'}
              </button>
              <span className={`binder-glyph binder-glyph-${item.kind}`} aria-hidden="true">
                {item.kind === 'folder' ? '▤' : '¶'}
              </span>

              {renamingId === item.id ? (
                <input
                  className="binder-rename"
                  value={draftTitle}
                  autoFocus
                  aria-label="Title"
                  onChange={(event) => setDraftTitle(event.target.value)}
                  onBlur={commitRename}
                  onClick={(event) => event.stopPropagation()}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      commitRename();
                    }
                    if (event.key === 'Escape') {
                      setRenamingId(null);
                    }
                  }}
                />
              ) : (
                <span className="binder-title" title={item.synopsis || item.title}>
                  {item.title}
                </span>
              )}

              {item.trashed && (
                <span className="binder-row-actions">
                  <button
                    type="button"
                    className="binder-row-action"
                    title="Restore"
                    onClick={(event) => {
                      event.stopPropagation();
                      onRestore(item.id);
                    }}
                  >
                    ↺
                  </button>
                  <button
                    type="button"
                    className="binder-row-action"
                    title="Delete permanently"
                    onClick={(event) => {
                      event.stopPropagation();
                      onDelete(item.id);
                    }}
                  >
                    ×
                  </button>
                </span>
              )}

              {!item.trashed && item.kind === 'document' && item.word_count > 0 && (
                <span className="binder-count">{item.word_count}</span>
              )}
            </div>
          );
        })}

        {rows.length === 0 && <p className="binder-empty">No item matches the filter.</p>}
      </div>

      <p className="binder-hint text-dim">
        {trashRoot
          ? 'Drag onto a folder to move an item. Double-click to rename.'
          : ''}
      </p>
    </section>
  );
}
