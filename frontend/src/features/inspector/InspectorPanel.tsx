/**
 * The inspector: the metadata of the selected item, and the reference
 * sheets of the project.
 *
 * Metadata is a full replacement: the form holds the complete state and
 * the Save button writes it, so clearing a field is expressible.
 */

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as api from '../../bindings';
import type { BinderItem, ItemMeta, ReferenceSheet } from '../../bindings';
import {
  fieldsToText,
  keywordsToInput,
  linkDocument,
  metaFromItem,
  newSheet,
  parseFields,
  parseKeywords,
  sheetKinds,
} from './inspector';
import type { SheetKind } from './inspector';

interface InspectorPanelProps {
  projectId: string;
  item: BinderItem | null;
  busy: boolean;
  onMeta: (itemId: string, meta: ItemMeta) => void;
  onSnapshot: (message: string) => void;
  onOpenHistory: () => void;
  notify: (message: string, kind?: 'info' | 'error' | 'success') => void;
}

/**
 * The inspector panel.
 */
export function InspectorPanel({
  projectId,
  item,
  busy,
  onMeta,
  onSnapshot,
  onOpenHistory,
  notify,
}: InspectorPanelProps) {
  const [tab, setTab] = useState<'meta' | 'refs'>('meta');
  const [draft, setDraft] = useState<ItemMeta | null>(item ? metaFromItem(item) : null);
  const [keywordsText, setKeywordsText] = useState(item ? keywordsToInput(item.keywords) : '');

  // Reload the form when the selected record changes, including after a
  // save from another view.
  useEffect(() => {
    setDraft(item ? metaFromItem(item) : null);
    setKeywordsText(item ? keywordsToInput(item.keywords) : '');
  }, [item?.id, item?.updated]);

  const dirty =
    item !== null &&
    draft !== null &&
    JSON.stringify({ ...draft, keywords: parseKeywords(keywordsText) }) !==
      JSON.stringify(metaFromItem(item));

  const save = () => {
    if (!item || !draft) {
      return;
    }
    onMeta(item.id, { ...draft, keywords: parseKeywords(keywordsText) });
  };

  return (
    <aside className="inspector" aria-label="Inspector">
      <div className="inspector-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'meta'}
          className={`inspector-tab${tab === 'meta' ? ' is-active' : ''}`}
          onClick={() => setTab('meta')}
        >
          Metadata
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={tab === 'refs'}
          className={`inspector-tab${tab === 'refs' ? ' is-active' : ''}`}
          onClick={() => setTab('refs')}
        >
          Reference
        </button>
      </div>

      {tab === 'meta' ? (
        !item || !draft ? (
          <p className="inspector-placeholder text-dim">
            Select an item to see its synopsis, notes and metadata.
          </p>
        ) : (
          <div className="inspector-body">
            <label className="field">
              <span className="field-label">Title</span>
              <input
                value={draft.title}
                disabled={busy}
                onChange={(event) => setDraft({ ...draft, title: event.target.value })}
              />
            </label>

            <label className="field">
              <span className="field-label">Synopsis</span>
              <textarea
                rows={4}
                value={draft.synopsis}
                placeholder="What this section is about…"
                onChange={(event) => setDraft({ ...draft, synopsis: event.target.value })}
              />
            </label>

            <label className="field">
              <span className="field-label">Notes</span>
              <textarea
                rows={5}
                value={draft.notes}
                placeholder="Private notes, not compiled…"
                onChange={(event) => setDraft({ ...draft, notes: event.target.value })}
              />
            </label>

            <label className="field">
              <span className="field-label">Keywords</span>
              <input
                value={keywordsText}
                placeholder="rain, night, storm"
                onChange={(event) => setKeywordsText(event.target.value)}
              />
              <span className="field-hint">Separated by commas.</span>
            </label>

            <div className="inspector-pair">
              <label className="field">
                <span className="field-label">Label</span>
                <input
                  value={draft.label}
                  placeholder="Chapter"
                  onChange={(event) => setDraft({ ...draft, label: event.target.value })}
                />
              </label>
              <label className="field">
                <span className="field-label">Status</span>
                <input
                  value={draft.status}
                  placeholder="First Draft"
                  onChange={(event) => setDraft({ ...draft, status: event.target.value })}
                />
              </label>
            </div>

            <label className="field field-inline">
              <input
                type="checkbox"
                checked={draft.include_in_compile}
                disabled={item.kind !== 'document'}
                onChange={(event) => setDraft({ ...draft, include_in_compile: event.target.checked })}
              />
              <span>Include in the compile</span>
            </label>

            <div className="inspector-actions">
              <button type="button" className="btn btn-sm btn-primary" disabled={!dirty || busy} onClick={save}>
                Save metadata
              </button>
              <button
                type="button"
                className="btn btn-sm"
                disabled={busy}
                onClick={() => onSnapshot(`Snapshot of ${item.title}`)}
              >
                Snapshot
              </button>
              <button type="button" className="btn btn-sm" onClick={onOpenHistory}>
                History
              </button>
            </div>

            <dl className="inspector-stats text-dim">
              <div>
                <dt>Words</dt>
                <dd>{item.word_count}</dd>
              </div>
              <div>
                <dt>Characters</dt>
                <dd>{item.char_count}</dd>
              </div>
              <div>
                <dt>Kind</dt>
                <dd>{item.kind}</dd>
              </div>
              <div>
                <dt>Bucket</dt>
                <dd>{item.bucket}</dd>
              </div>
              <div>
                <dt>Revision</dt>
                <dd>{item.last_commit ? item.last_commit.slice(0, 7) : '—'}</dd>
              </div>
            </dl>
          </div>
        )
      ) : (
        <ReferenceTab projectId={projectId} item={item} busy={busy} notify={notify} />
      )}
    </aside>
  );
}

interface ReferenceTabProps {
  projectId: string;
  item: BinderItem | null;
  busy: boolean;
  notify: (message: string, kind?: 'info' | 'error' | 'success') => void;
}

/**
 * ReferenceTab lists the character, location and item sheets and edits
 * one of them.
 */
function ReferenceTab({ projectId, item, busy, notify }: ReferenceTabProps) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<ReferenceSheet | null>(null);
  const [fieldsText, setFieldsText] = useState('');

  const sheetsQuery = useQuery({
    queryKey: ['reference-sheets', projectId],
    queryFn: () => api.listReferenceSheets(projectId),
  });

  const saveMutation = useMutation({
    mutationFn: (sheet: ReferenceSheet) => api.saveReferenceSheet(sheet),
    onSuccess: (saved) => {
      setDraft(saved);
      setFieldsText(fieldsToText(saved.fields));
      void queryClient.invalidateQueries({ queryKey: ['reference-sheets', projectId] });
      notify('The reference sheet was saved.', 'success');
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  const deleteMutation = useMutation({
    mutationFn: (sheetId: string) => api.deleteReferenceSheet(sheetId),
    onSuccess: () => {
      setDraft(null);
      void queryClient.invalidateQueries({ queryKey: ['reference-sheets', projectId] });
      notify('The reference sheet was removed.');
    },
    onError: (error: Error) => notify(error.message, 'error'),
  });

  const openSheet = (sheet: ReferenceSheet) => {
    setDraft(sheet);
    setFieldsText(fieldsToText(sheet.fields));
  };

  const sheets = sheetsQuery.data ?? [];

  return (
    <div className="inspector-body">
      <div className="inspector-actions">
        {(sheetKinds as readonly SheetKind[]).map((kind) => (
          <button
            key={kind}
            type="button"
            className="btn btn-sm"
            disabled={busy}
            onClick={() => {
              setDraft(newSheet(projectId, item, kind));
              setFieldsText('');
            }}
          >
            + {kind}
          </button>
        ))}
      </div>

      {sheetsQuery.error ? (
        <p className="text-dim">The reference sheets could not be loaded.</p>
      ) : (
        <ul className="sheet-list">
          {sheets.map((sheet) => (
            <li key={sheet.id} className="sheet-row">
              <button
                type="button"
                className={`sheet-open${draft?.id === sheet.id ? ' is-selected' : ''}`}
                onClick={() => openSheet(sheet)}
              >
                <span className="sheet-kind">{sheet.kind}</span>
                <strong>{sheet.name}</strong>
                {sheet.summary && <span className="text-dim">{sheet.summary}</span>}
              </button>
              <button
                type="button"
                className="binder-row-action"
                title="Delete the sheet"
                disabled={busy}
                onClick={() => deleteMutation.mutate(sheet.id)}
              >
                ×
              </button>
            </li>
          ))}
          {sheets.length === 0 && <li className="text-dim">No reference sheets yet.</li>}
        </ul>
      )}

      {draft && (
        <div className="sheet-editor">
          <label className="field">
            <span className="field-label">Name</span>
            <input
              value={draft.name}
              onChange={(event) => setDraft({ ...draft, name: event.target.value })}
            />
          </label>

          <label className="field">
            <span className="field-label">Kind</span>
            <select
              value={draft.kind}
              onChange={(event) => setDraft({ ...draft, kind: event.target.value })}
            >
              {(sheetKinds as readonly string[]).map((kind) => (
                <option key={kind} value={kind}>
                  {kind}
                </option>
              ))}
            </select>
          </label>

          <label className="field">
            <span className="field-label">Summary</span>
            <textarea
              rows={3}
              value={draft.summary}
              onChange={(event) => setDraft({ ...draft, summary: event.target.value })}
            />
          </label>

          <label className="field">
            <span className="field-label">Notes</span>
            <textarea
              rows={4}
              value={draft.notes}
              onChange={(event) => setDraft({ ...draft, notes: event.target.value })}
            />
          </label>

          <label className="field">
            <span className="field-label">Fields</span>
            <textarea
              rows={4}
              value={fieldsText}
              placeholder={'age = 34\nrole = lighthouse keeper'}
              onChange={(event) => setFieldsText(event.target.value)}
            />
            <span className="field-hint">One “key = value” per line.</span>
          </label>

          <p className="text-dim">
            Linked to {draft.document_ids.length} document
            {draft.document_ids.length === 1 ? '' : 's'}.
          </p>

          <div className="inspector-actions">
            <button
              type="button"
              className="btn btn-sm btn-primary"
              disabled={busy}
              onClick={() => saveMutation.mutate({ ...draft, fields: parseFields(fieldsText) })}
            >
              Save sheet
            </button>
            <button
              type="button"
              className="btn btn-sm"
              disabled={busy || !item || item.kind !== 'document'}
              onClick={() => setDraft(linkDocument(draft, item ? item.id : ''))}
            >
              Link this document
            </button>
            <button type="button" className="btn btn-sm" onClick={() => setDraft(null)}>
              Close
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
