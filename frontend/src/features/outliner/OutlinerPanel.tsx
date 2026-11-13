/**
 * The outliner: a sortable, editable table over the binder subtree.
 *
 * The cell editors write on blur through the inspector's metadata
 * mutation, so the outliner, the inspector and the binder always show
 * the same record.
 */

import { useEffect, useMemo, useState } from 'react';

import type { BinderItem, ItemMeta } from '../../bindings';
import { outlineRoot, outlineRows, sortRows, targetProgress, totalWords } from './outliner';
import type { OutlineColumn, SortDirection } from './outliner';

interface OutlinerPanelProps {
  items: BinderItem[];
  selectedId: string;
  busy: boolean;
  /** The draft word target, shown as a progress bar. Zero means no goal. */
  target: number;
  onSelect: (itemId: string) => void;
  onOpen: (itemId: string) => void;
  onMeta: (itemId: string, patch: Partial<ItemMeta>) => void;
}

/**
 * The outliner panel.
 */
export function OutlinerPanel({
  items,
  selectedId,
  busy,
  target,
  onSelect,
  onOpen,
  onMeta,
}: OutlinerPanelProps) {
  const [column, setColumn] = useState<OutlineColumn>('title');
  const [direction, setDirection] = useState<SortDirection>('asc');

  const rootId = outlineRoot(items, selectedId);
  const rows = useMemo(() => outlineRows(items, rootId), [items, rootId]);
  const sorted = useMemo(() => sortRows(rows, column, direction), [rows, column, direction]);
  const words = totalWords(rows);
  const progress = targetProgress(words, target);

  const header = (key: OutlineColumn, label: string) => (
    <button
      type="button"
      className={`outline-sort${column === key ? ' is-active' : ''}`}
      onClick={() => {
        if (column === key) {
          setDirection(direction === 'asc' ? 'desc' : 'asc');
        } else {
          setColumn(key);
          setDirection('asc');
        }
      }}
    >
      {label}
      {column === key ? (direction === 'asc' ? ' ▲' : ' ▼') : ''}
    </button>
  );

  return (
    <div className="outliner">
      <div className="outliner-header">
        <div>
          <h2 className="editor-title">Outline</h2>
          <p className="editor-subtitle text-dim">
            {rows.length} item{rows.length === 1 ? '' : 's'} · {words} words
          </p>
        </div>
        {target > 0 && (
          <div className="outliner-progress" title={`${words} of ${target} words`}>
            <div className="outliner-progress-bar" style={{ width: `${progress}%` }} />
            <span>{progress}%</span>
          </div>
        )}
      </div>

      <div className="outline-table" role="table" aria-label="Outliner">
        <div className="outline-row outline-head" role="row">
          <span role="columnheader" className="outline-cell outline-title">
            {header('title', 'Title')}
          </span>
          <span role="columnheader" className="outline-cell outline-synopsis">
            {header('synopsis', 'Synopsis')}
          </span>
          <span role="columnheader" className="outline-cell outline-label">
            {header('label', 'Label')}
          </span>
          <span role="columnheader" className="outline-cell outline-status">
            {header('status', 'Status')}
          </span>
          <span role="columnheader" className="outline-cell outline-words">
            {header('words', 'Words')}
          </span>
          <span role="columnheader" className="outline-cell outline-include">
            Compile
          </span>
        </div>

        {sorted.map((row) => (
          <div
            key={row.item.id}
            role="row"
            className={`outline-row${row.item.id === selectedId ? ' is-selected' : ''}${
              row.item.trashed ? ' is-trashed' : ''
            }`}
            onClick={() => onSelect(row.item.id)}
            onDoubleClick={() => onOpen(row.item.id)}
          >
            <span className="outline-cell outline-title" style={{ paddingLeft: `${0.35 + row.depth * 0.9}rem` }}>
              <span className={`binder-glyph binder-glyph-${row.item.kind}`} aria-hidden="true">
                {row.item.kind === 'folder' ? '▤' : '¶'}
              </span>
              <EditableCell
                value={row.item.title}
                label={`Title of ${row.item.title}`}
                disabled={busy}
                onCommit={(value) => onMeta(row.item.id, { title: value })}
              />
            </span>
            <span className="outline-cell outline-synopsis">
              <EditableCell
                value={row.item.synopsis}
                label={`Synopsis of ${row.item.title}`}
                placeholder="Synopsis…"
                disabled={busy}
                onCommit={(value) => onMeta(row.item.id, { synopsis: value })}
              />
            </span>
            <span className="outline-cell outline-label">
              <EditableCell
                value={row.item.label}
                label={`Label of ${row.item.title}`}
                placeholder="Label"
                disabled={busy}
                onCommit={(value) => onMeta(row.item.id, { label: value })}
              />
            </span>
            <span className="outline-cell outline-status">
              <EditableCell
                value={row.item.status}
                label={`Status of ${row.item.title}`}
                placeholder="Status"
                disabled={busy}
                onCommit={(value) => onMeta(row.item.id, { status: value })}
              />
            </span>
            <span className="outline-cell outline-words">
              {row.item.kind === 'document' ? row.item.word_count : '—'}
            </span>
            <span className="outline-cell outline-include">
              <input
                type="checkbox"
                checked={row.item.include_in_compile}
                disabled={busy || row.item.kind !== 'document'}
                aria-label={`Include ${row.item.title} in the compile`}
                onClick={(event) => event.stopPropagation()}
                onChange={(event) => onMeta(row.item.id, { include_in_compile: event.target.checked })}
              />
            </span>
          </div>
        ))}

        {sorted.length === 0 && (
          <p className="outliner-empty text-dim">This folder has no sections yet.</p>
        )}
      </div>
    </div>
  );
}

interface EditableCellProps {
  value: string;
  label: string;
  placeholder?: string;
  disabled: boolean;
  onCommit: (value: string) => void;
}

/**
 * EditableCell is a text input that commits on blur or Enter and resets
 * on Escape.
 */
function EditableCell({ value, label, placeholder, disabled, onCommit }: EditableCellProps) {
  const [draft, setDraft] = useState(value);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  const commit = () => {
    if (draft.trim() !== value) {
      onCommit(draft.trim());
    }
  };

  return (
    <input
      className="outline-input"
      value={draft}
      aria-label={label}
      placeholder={placeholder}
      disabled={disabled}
      onClick={(event) => event.stopPropagation()}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.currentTarget.blur();
        }
        if (event.key === 'Escape') {
          setDraft(value);
          event.currentTarget.blur();
        }
      }}
    />
  );
}
