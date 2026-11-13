/**
 * The corkboard: one index card per binder item, with the synopsis
 * editable on the card and drag-and-drop reordering.
 */

import { useEffect, useState } from 'react';

import type { BinderItem } from '../../bindings';
import { cardsFor, cardsWords, cardDrop, folderTitle } from './corkboard';

interface CorkboardPanelProps {
  items: BinderItem[];
  selectedId: string;
  busy: boolean;
  onSelect: (itemId: string) => void;
  onOpen: (itemId: string) => void;
  onSynopsis: (itemId: string, synopsis: string) => void;
  onMove: (itemId: string, parentId: string, index: number) => void;
}

/**
 * The corkboard panel.
 */
export function CorkboardPanel({
  items,
  selectedId,
  busy,
  onSelect,
  onOpen,
  onSynopsis,
  onMove,
}: CorkboardPanelProps) {
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const cards = cardsFor(items, selectedId);

  return (
    <div className="corkboard">
      <div className="corkboard-header">
        <h2 className="editor-title">{folderTitle(items, selectedId)}</h2>
        <p className="editor-subtitle text-dim">
          {cards.length} card{cards.length === 1 ? '' : 's'} · {cardsWords(cards)} words
        </p>
      </div>

      {cards.length === 0 ? (
        <p className="editor-placeholder text-dim">
          Select a folder with sections to see its index cards.
        </p>
      ) : (
        <div className="corkboard-grid">
          {cards.map((card) => (
            <IndexCard
              key={card.item.id}
              item={card.item}
              childCount={card.children}
              selected={card.item.id === selectedId}
              busy={busy}
              dragging={draggingId === card.item.id}
              onDragStart={() => setDraggingId(card.item.id)}
              onDragEnd={() => setDraggingId(null)}
              onDropOn={(targetId) => {
                if (!draggingId) {
                  return;
                }
                const target = cardDrop(items, draggingId, targetId);
                if (target) {
                  onMove(draggingId, target.parentID, target.index);
                }
                setDraggingId(null);
              }}
              onSelect={() => onSelect(card.item.id)}
              onOpen={() => onOpen(card.item.id)}
              onSynopsis={(value) => onSynopsis(card.item.id, value)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface IndexCardProps {
  item: BinderItem;
  childCount: number;
  selected: boolean;
  busy: boolean;
  dragging: boolean;
  onDragStart: () => void;
  onDragEnd: () => void;
  onDropOn: (targetId: string) => void;
  onSelect: () => void;
  onOpen: () => void;
  onSynopsis: (synopsis: string) => void;
}

/**
 * One index card.
 */
function IndexCard({
  item,
  childCount,
  selected,
  busy,
  dragging,
  onDragStart,
  onDragEnd,
  onDropOn,
  onSelect,
  onOpen,
  onSynopsis,
}: IndexCardProps) {
  const [synopsis, setSynopsis] = useState(item.synopsis);

  // The record is the source of truth: when another view changes the
  // synopsis, the card follows it.
  useEffect(() => {
    setSynopsis(item.synopsis);
  }, [item.synopsis, item.id]);

  return (
    <article
      className={`index-card${selected ? ' is-selected' : ''}${dragging ? ' is-dragging' : ''}`}
      draggable={!busy}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragOver={(event) => event.preventDefault()}
      onDrop={(event) => {
        event.preventDefault();
        event.stopPropagation();
        onDropOn(item.id);
      }}
      onClick={onSelect}
      onDoubleClick={onOpen}
    >
      <header className="index-card-header">
        <span className={`binder-glyph binder-glyph-${item.kind}`} aria-hidden="true">
          {item.kind === 'folder' ? '▤' : '¶'}
        </span>
        <h3 className="index-card-title">{item.title}</h3>
        {item.kind === 'folder' && <span className="index-card-count">{childCount}</span>}
      </header>

      <textarea
        className="index-card-synopsis"
        value={synopsis}
        placeholder="Synopsis…"
        aria-label={`Synopsis of ${item.title}`}
        onClick={(event) => event.stopPropagation()}
        onChange={(event) => setSynopsis(event.target.value)}
        onBlur={() => {
          if (synopsis !== item.synopsis) {
            onSynopsis(synopsis);
          }
        }}
      />

      <footer className="index-card-footer text-dim">
        <span>{item.kind === 'document' ? `${item.word_count} words` : `${childCount} items`}</span>
        {item.label && <span className="binder-label">{item.label}</span>}
      </footer>
    </article>
  );
}
