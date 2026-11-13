/**
 * The writing surface.
 *
 * One document is edited in a Markdown textarea with a formatting
 * toolbar, live counts and an idle autosave. A folder opens as a
 * Scrivenings view: each document below it is a separately saved
 * section, so editing across several sections stays explicit.
 */

import { useEffect, useRef, useState } from 'react';

import type { BinderItem, Document, Section, Scrivenings } from '../../bindings';
import {
  applyFormat,
  countCharacters,
  countWords,
  findMatches,
  readingMinutes,
  replaceAllText,
} from './editor';
import type { FormatAction } from './editor';
import { registerFlusher } from './flush';

interface EditorPanelProps {
  /** The selected item, or null. */
  item: BinderItem | null;
  /** The selected document, if the item is one. */
  document: Document | null;
  /** The sections of a folder, if the item is a folder. */
  scrivenings: Scrivenings | null;
  /** True while the document is being loaded. */
  loading: boolean;
  /** True while a save is in flight. */
  saving: boolean;
  /** Navigates to another binder item. */
  onOpenItem: (itemId: string) => void;
  /** Saves one document body. */
  onSave: (itemId: string, content: string) => Promise<unknown>;
  /** Takes a named snapshot of the project. */
  onSnapshot: (message: string) => void;
}

/**
 * The editor panel.
 */
export function EditorPanel({
  item,
  document: doc,
  scrivenings,
  loading,
  saving,
  onOpenItem,
  onSave,
  onSnapshot,
}: EditorPanelProps) {
  if (loading) {
    return (
      <div className="editor">
        <p className="editor-placeholder text-dim">Loading the document…</p>
      </div>
    );
  }

  if (scrivenings) {
    return (
      <div className="editor editor-scrivenings">
        <div className="editor-header">
          <div>
            <h2 className="editor-title">{scrivenings.title}</h2>
            <p className="editor-subtitle text-dim">
              {scrivenings.sections.length} section
              {scrivenings.sections.length === 1 ? '' : 's'} · Scrivenings view
            </p>
          </div>
          <button
            type="button"
            className="btn btn-sm"
            onClick={() => onSnapshot(`Snapshot of ${scrivenings.title}`)}
          >
            Snapshot
          </button>
        </div>

        {scrivenings.sections.map((section) => (
          <section className="scrivening" key={section.item_id}>
            <button
              type="button"
              className="scrivening-title"
              title="Open this section on its own"
              onClick={() => onOpenItem(section.item_id)}
            >
              {section.title}
            </button>
            <SectionEditor section={section} saving={saving} onSave={onSave} />
          </section>
        ))}

        {scrivenings.sections.length === 0 && (
          <p className="editor-placeholder text-dim">
            This folder has no documents yet. Add one from the binder.
          </p>
        )}
      </div>
    );
  }

  if (!doc) {
    return (
      <div className="editor">
        <p className="editor-placeholder text-dim">
          {item?.trashed
            ? 'This item is in the Trash. Restore it from the binder to edit it again.'
            : item
              ? 'This folder is empty. Add a document, or open the Corkboard to see its cards.'
              : 'Select a document in the binder, or create one.'}
        </p>
      </div>
    );
  }

  return (
    <DocumentEditor
      key={doc.item.id}
      item={doc.item}
      initialContent={doc.content}
      revision={doc.revision}
      saving={saving}
      onSave={onSave}
      onSnapshot={onSnapshot}
    />
  );
}

/**
 * useAutosave keeps the buffer and saves it after an idle pause.
 *
 * The save also runs on unmount, so switching documents within the pause
 * window does not lose the last sentence.
 */
function useAutosave(
  itemId: string,
  initialContent: string,
  onSave: (itemId: string, content: string) => Promise<unknown>,
): { text: string; setText: (text: string) => void; dirty: boolean } {
  const [text, setText] = useState(initialContent);
  const dirtyRef = useRef(false);
  const textRef = useRef(text);
  const saveRef = useRef(onSave);
  saveRef.current = onSave;

  useEffect(() => {
    textRef.current = text;
    if (text === initialContent) {
      dirtyRef.current = false;
      return;
    }
    dirtyRef.current = true;
    const timer = window.setTimeout(() => {
      // Keep the buffer dirty until the save actually resolves, so a
      // failed save is retried by the next autosave or by the close
      // handshake instead of being silently dropped.
      const pending = saveRef.current(itemId, text);
      void Promise.resolve(pending).then(
        () => {
          dirtyRef.current = false;
        },
        () => {
          dirtyRef.current = true;
        },
      );
    }, 900);
    return () => window.clearTimeout(timer);
  }, [text, initialContent, itemId]);

  useEffect(
    () => () => {
      if (dirtyRef.current) {
        void saveRef.current(itemId, textRef.current);
      }
    },
    [itemId],
  );

  // The close handshake asks every mounted editor to save. Without it,
  // text typed inside the 900 ms autosave pause is lost when the window
  // closes, because the webview is destroyed before the timer fires.
  useEffect(
    () =>
      registerFlusher(() => {
        if (!dirtyRef.current) {
          return Promise.resolve();
        }
        return Promise.resolve()
          .then(() => saveRef.current(itemId, textRef.current))
          .then(() => {
            dirtyRef.current = false;
          });
      }),
    [itemId],
  );

  return { text, setText, dirty: text !== initialContent };
}

interface DocumentEditorProps {
  item: BinderItem;
  initialContent: string;
  revision: string;
  saving: boolean;
  onSave: (itemId: string, content: string) => Promise<unknown>;
  onSnapshot: (message: string) => void;
}

/**
 * The editor of one document.
 */
function DocumentEditor({
  item,
  initialContent,
  revision,
  saving,
  onSave,
  onSnapshot,
}: DocumentEditorProps) {
  const { text, setText, dirty } = useAutosave(item.id, initialContent, onSave);
  const areaRef = useRef<HTMLTextAreaElement | null>(null);
  const [showFind, setShowFind] = useState(false);
  const [find, setFind] = useState('');
  const [replacement, setReplacement] = useState('');
  const [snapshotMessage, setSnapshotMessage] = useState('');

  const words = countWords(text);
  const matches = findMatches(text, find);

  const format = (action: FormatAction) => {
    const area = areaRef.current;
    if (!area) {
      return;
    }
    const result = applyFormat(
      { text, selectionStart: area.selectionStart, selectionEnd: area.selectionEnd },
      action,
    );
    setText(result.text);
    window.requestAnimationFrame(() => {
      const next = areaRef.current;
      if (next) {
        next.focus();
        next.setSelectionRange(result.selectionStart, result.selectionEnd);
      }
    });
  };

  const saveNow = () => {
    if (dirty) {
      void onSave(item.id, text);
    }
  };

  return (
    <div className="editor">
      <div className="editor-header">
        <div className="editor-heading">
          <h2 className="editor-title">{item.title}</h2>
          <p className="editor-subtitle text-dim">
            {dirty ? 'Unsaved changes' : saving ? 'Saving…' : 'Saved'} · revision{' '}
            {revision ? revision.slice(0, 7) : '—'}
          </p>
        </div>
        <div className="editor-actions">
          <button type="button" className="btn btn-sm" onClick={() => setShowFind((value) => !value)}>
            Find
          </button>
          <button type="button" className="btn btn-sm" onClick={saveNow} disabled={!dirty}>
            Save now
          </button>
        </div>
      </div>

      <div className="editor-formatbar">
        {(
          [
            ['bold', 'B', 'Bold'],
            ['italic', 'I', 'Italic'],
            ['code', '</>', 'Inline code'],
            ['h1', 'H1', 'Heading 1'],
            ['h2', 'H2', 'Heading 2'],
            ['quote', '❝', 'Block quote'],
            ['bullet', '•', 'Bulleted list'],
            ['numbered', '1.', 'Numbered list'],
          ] as Array<[FormatAction, string, string]>
        ).map(([action, label, title]) => (
          <button
            key={action}
            type="button"
            className="editor-format"
            title={title}
            onMouseDown={(event) => {
              // Keep the textarea's selection while the button is clicked.
              event.preventDefault();
              format(action);
            }}
          >
            {label}
          </button>
        ))}

        <span className="editor-format-spacer" />
        <input
          className="editor-snapshot-input"
          type="text"
          value={snapshotMessage}
          placeholder="Snapshot message"
          aria-label="Snapshot message"
          onChange={(event) => setSnapshotMessage(event.target.value)}
        />
        <button
          type="button"
          className="btn btn-sm"
          onClick={() => {
            onSnapshot(snapshotMessage.trim() || `Snapshot of ${item.title}`);
            setSnapshotMessage('');
          }}
        >
          Snapshot
        </button>
      </div>

      {showFind && (
        <div className="editor-findbar">
          <input
            type="search"
            placeholder="Find"
            aria-label="Find"
            value={find}
            onChange={(event) => setFind(event.target.value)}
          />
          <input
            type="text"
            placeholder="Replace with"
            aria-label="Replace with"
            value={replacement}
            onChange={(event) => setReplacement(event.target.value)}
          />
          <span className="text-dim">{matches.length} match(es)</span>
          <button
            type="button"
            className="btn btn-sm"
            disabled={!find}
            onClick={() => setText(replaceAllText(text, find, replacement))}
          >
            Replace all
          </button>
        </div>
      )}

      <textarea
        ref={areaRef}
        className="editor-text"
        value={text}
        spellCheck
        aria-label={`Body of ${item.title}`}
        placeholder="Write the section in Markdown…"
        onChange={(event) => setText(event.target.value)}
        onKeyDown={(event) => {
          if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
            event.preventDefault();
            saveNow();
          }
        }}
      />

      <div className="editor-status">
        <span>{words} words</span>
        <span>{countCharacters(text)} characters</span>
        <span>{countCharacters(text, true)} with spaces</span>
        <span>{readingMinutes(words)} min read</span>
      </div>
    </div>
  );
}

interface SectionEditorProps {
  section: Section;
  saving: boolean;
  onSave: (itemId: string, content: string) => Promise<unknown>;
}

/**
 * One editable section of a Scrivenings view.
 */
function SectionEditor({ section, saving, onSave }: SectionEditorProps) {
  const { text, setText, dirty } = useAutosave(section.item_id, section.content, onSave);
  return (
    <div className="scrivening-body">
      <textarea
        className="editor-text"
        value={text}
        spellCheck
        aria-label={`Body of ${section.title}`}
        onChange={(event) => setText(event.target.value)}
      />
      <div className="editor-status">
        <span>{countWords(text)} words</span>
        <span>{dirty ? 'Unsaved changes' : saving ? 'Saving…' : 'Saved'}</span>
      </div>
    </div>
  );
}
